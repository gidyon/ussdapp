package ussdapp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	currentMenuKey = "current_menu"
	currentPayload = "current_payload"
	languageKey    = "language"
)

type UssdApp struct {
	homeMenu string
	allmenus map[string]Menu
	menus    []string
	opt      *Options
}

// Options contains data required for ussd app
type Options struct {
	AppName         string
	Cache           *redis.Client
	Logger          Logger
	TableName       string
	DefaultLanguage string
	SessionDuration time.Duration
}

type SessionResponse struct {
	Response      string
	Failed        bool
	StatusMessage string
	MenuName      string
}

func NewUssdApp(ctx context.Context, opt *Options) (*UssdApp, error) {
	// Validation
	switch {
	case opt == nil:
		return nil, errors.New("missing options")
	case opt.AppName == "":
		return nil, errors.New("missing app name")
	case opt.Cache == nil:
		return nil, errors.New("missing redis db")
	case opt.Logger == nil:
		return nil, errors.New("missing logger")
	default:
		if opt.SessionDuration == 0 {
			opt.SessionDuration = time.Minute * 10
		}
	}

	app := &UssdApp{
		homeMenu: "",
		allmenus: make(map[string]Menu),
		menus:    []string{},
		opt:      opt,
	}

	_ = ctx

	return app, nil
}

func ValidateAppMenus(app *UssdApp) error {

	for _, val := range app.allmenus {
		_, ok := app.allmenus[val.MenuName()]
		if !ok {
			return fmt.Errorf("menu %s not registered", val.PreviousMenu())
		}
		_, ok = app.allmenus[val.PreviousMenu()]
		if !ok && val.PreviousMenu() != "" && app.homeMenu != val.MenuName() {
			return fmt.Errorf("previous menu %s for %s menu is not registered", val.PreviousMenu(), val.MenuName())
		}
		_, ok = app.allmenus[val.NextMenu()]
		if !ok && val.NextMenu() != "" {
			return fmt.Errorf("next menu %s for %s menu is not registered", val.NextMenu(), val.MenuName())
		}
		for _, menuItem := range val.(*menu).menuItems {
			_, ok = app.allmenus[menuItem]
			if !ok {
				return fmt.Errorf("next item %s for %s menu is not registered", menuItem, val.MenuName())
			}
		}
	}

	return nil
}

func ValidateMenu(m Menu) error {
	// Validate menu
	switch {
	case m == nil:
		return errors.New("nil menu not allowed")
	case m.MenuName() == "":
		return errors.New("missing menu name")
	case m.PreviousMenu() == "":
		return fmt.Errorf("previous menu for %s is missing", m.MenuName())
	case m.NextMenu() == "" && m.HasArbitraryInput():
		return fmt.Errorf("next menu for %s is missing", m.MenuName())
	case m.(*menu).menuItems == nil && !m.HasArbitraryInput():
		return fmt.Errorf("missing menu items for menu %s", m.MenuName())
	}
	return nil
}

func (app *UssdApp) AddMenu(m Menu) error {
	err := ValidateMenu(m)
	if err != nil {
		return err
	}

	_, ok := app.allmenus[m.MenuName()]
	if ok {
		return fmt.Errorf("%w: %s", ErrMenuExist, m.MenuName())
	}

	app.allmenus[m.MenuName()] = m

	app.menus = append(app.menus, m.MenuName())

	app.opt.Logger.Infof("registered %s menu", m.MenuName())

	return nil
}

var (
	mu               = &sync.RWMutex{} // guards validationErrors
	validationErrors = make(map[string]string, 100)
)

func getValidationError(up UssdPayload) string {
	key := fmt.Sprintf("%s:%s", up.Msisdn(), up.SessionId())
	mu.Lock()
	err, ok := validationErrors[key]
	if ok {
		delete(validationErrors, key)
	}
	mu.Unlock()
	return err
}

func addValidationError(validationErr string, up UssdPayload) {
	mu.Lock()
	up.(*ussdPayload).validationFailed = true
	validationErrors[fmt.Sprintf("%s:%s", up.Msisdn(), up.SessionId())] = validationErr
	mu.Unlock()
}

// SuccessfulResponse is a helper that will return successful *SessionResponse i.e the SessionResponse.Failed will be fa,se;
func SuccessfulResponse(sessionResponse *SessionResponse) *SessionResponse {
	if sessionResponse == nil {
		sessionResponse = &SessionResponse{}
	}
	sessionResponse.Failed = false
	return sessionResponse
}

// FailedResponse is a helper that will return failed *SessionResponse i.e the SessionResponse.Failed will be true;
func FailedResponse(sessionResponse *SessionResponse) *SessionResponse {
	if sessionResponse == nil {
		sessionResponse = &SessionResponse{}
	}
	sessionResponse.Failed = true
	return sessionResponse
}

func (app *UssdApp) Cache() *redis.Client {
	return app.opt.Cache
}

func (app *UssdApp) sessionKey(payload UssdPayload) string {
	return fmt.Sprintf("%s:sessions:%s:%s", app.opt.AppName, payload.SessionId(), payload.Msisdn())
}

// SetHomeMenu will set default home menu for the ussd app
func (app *UssdApp) SetHomeMenu(menuName string) {
	app.homeMenu = menuName
}

// GetMenuNames will return all menu names registered as a slice of strings
func (app *UssdApp) GetMenuNames() []string {
	return app.menus
}

// GetNextMenu will attempt to get the highest matching menu to be saved or/and rendered
func (app *UssdApp) GetNextMenu(payload UssdPayload, currentMenu string) (Menu, error) {
	currMenu, ok := app.allmenus[currentMenu]
	if !ok {
		return nil, fmt.Errorf("%v: %s", ErrMenuNotExist, currentMenu)
	}

	nextMenu, ok := currMenu.(*menu).menuItems[payload.UssdCurrentParam()]
	if !ok {
		if !currMenu.HasArbitraryInput() {
			// Add validation error
			addValidationError("INCORRECT input", payload)
			// return currMenu, nil
			return currMenu, ErrFailedValidation
		} else {
			next, ok := app.allmenus[currMenu.NextMenu()]
			if !ok {
				return nil, fmt.Errorf("%v: %s", ErrMenuNotExist, currentMenu)
			}
			return next, nil
		}
	}

	next, ok := app.allmenus[nextMenu]
	if !ok {
		return nil, fmt.Errorf("%v: %s", ErrMenuNotExist, currentMenu)
	}

	return next, nil
}

// SaveCurrentMenu will save the menu as current in cache.
//
// It will be used for the next incoming request to determine the right menu to render
//
// Will fail of the menu does not exist
func (app *UssdApp) SaveCurrentMenu(ctx context.Context, payload UssdPayload, menuName string) (Menu, error) {
	m, ok := app.allmenus[menuName]
	if !ok {
		return nil, ErrMenuNotExist
	}

	bs, err := payload.JSON()
	if err != nil {
		return nil, err
	}

	key := app.sessionKey(payload)
	_, err = app.opt.Cache.HMSet(ctx, key, currentMenuKey, m.MenuName(), currentPayload, bs).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to set current_menu and payload to map: %v", err)
	}

	app.opt.Logger.Infof("SAVED menu [%s] payload [%s]", m.MenuName(), payload.UssdCurrentParam)

	return m, nil
}

// GetCurrentMenu will get the current menu
//
// If no menu is found, it will default to home menu
//
// To set the home menu, use the helper SetHomeMenu
func (app *UssdApp) GetCurrentMenu(ctx context.Context, payload UssdPayload) (Menu, error) {
	res, err := app.opt.Cache.HGet(ctx, app.sessionKey(payload), currentMenuKey).Result()
	switch {
	case err == nil:
	case errors.Is(err, redis.Nil):
		return app.allmenus[app.homeMenu], nil
	default:
		return nil, fmt.Errorf("failed to get current_menu from map: %v", err)
	}

	menu, ok := app.allmenus[res]
	if !ok {
		return app.allmenus[app.homeMenu], nil
	}

	return menu, nil
}

// SetUserInCache is a helper to save user details in cache
func (app *UssdApp) SetUserInCache(ctx context.Context, payload UssdPayload, user map[string]interface{}) error {
	user["exists"] = "yes"
	_, err := app.opt.Cache.HSet(ctx, app.sessionKey(payload), user).Result()
	if err != nil {
		return fmt.Errorf("failed to set user in cache: %v", err)
	}
	return nil
}

// UserExistInCache is a helper to check if user is already saved in cache
func (app *UssdApp) UserExistInCache(ctx context.Context, payload UssdPayload) (bool, error) {
	v, err := app.opt.Cache.HExists(ctx, app.sessionKey(payload), "exists").Result()
	if err != nil {
		return false, fmt.Errorf("failed to check if user exists in cache: %v", err)
	}
	return v, nil
}

func sessionSetKey(appId string) string {
	return fmt.Sprintf("%s:sessions", appId)
}

// IsNewSession will check if incoming ussd session is new
//
// If session is new, it will be saved and automatically be cleared after session duration
func (app *UssdApp) IsNewSession(ctx context.Context, payload UssdPayload) (bool, error) {
	v, err := app.opt.Cache.SAdd(ctx, sessionSetKey(app.opt.AppName), fmt.Sprintf("%s:%s", payload.Msisdn(), payload.SessionId())).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check if session is new: %v", err)
	}

	var (
		sessionID = payload.SessionId()
		msisdn    = payload.Msisdn()
	)

	if v == 1 {
		time.AfterFunc(app.opt.SessionDuration, func() {
			// Delete user cache
			key := app.sessionKey(payload)
			err = app.Cache().Del(context.Background(), key).Err()
			if err != nil {
				app.opt.Logger.Errorf("ERROR DELETING SESSION DATA: %v", err)
			}
			// Remove session key from set
			err := app.deleteSessionSetKey(context.Background(), msisdn, sessionID)
			if err != nil {
				app.opt.Logger.Errorf("ERROR DELETING SESSION: %v", err)
				return
			}
			app.opt.Logger.Infof("SESSION %s DELETED", sessionID)
		})
	}

	return v == 1 || payload.UssdParams() == "", nil
}

// deleteSessionSetKey will remove session key from cache
func (app *UssdApp) deleteSessionSetKey(ctx context.Context, sessionID, msisdn string) error {
	err := app.opt.Cache.SRem(ctx, sessionSetKey(app.opt.AppName), fmt.Sprintf("%s:%s", msisdn, sessionID)).Err()
	if err != nil {
		return fmt.Errorf("failed to remove session: %v", err)
	}
	return nil
}

// SaveLanguage will save user language for the ussd session
func (app *UssdApp) SaveLanguage(ctx context.Context, payload UssdPayload, language string) error {
	err := app.opt.Cache.HSet(ctx, app.sessionKey(payload), languageKey, language).Err()
	if err != nil {
		return fmt.Errorf("failed to save language")
	}
	return nil
}

// GetLanguage will get the preferred language for the ussd session
func (app *UssdApp) GetLanguage(ctx context.Context, payload UssdPayload) string {
	lang, err := app.opt.Cache.HGet(ctx, app.sessionKey(payload), languageKey).Result()
	if err != nil {
		return app.opt.DefaultLanguage
	}
	return lang
}

// GetSessionKey will get the session key that is used to save the user data in cache
func (app *UssdApp) GetSessionKey(_ context.Context, payload UssdPayload) string {
	return app.sessionKey(payload)
}

func firstVal(vals ...string) string {
	for _, val := range vals {
		if val != "" {
			return val
		}
	}
	return ""
}

// GetPreviousPayload will get the payload for the ussd requesr
func (app *UssdApp) GetPreviousPayload(ctx context.Context, payload UssdPayload) (UssdPayload, error) {
	v, err := app.opt.Cache.HGet(ctx, app.sessionKey(payload), currentPayload).Result()
	switch {
	case err == nil:
	case errors.Is(err, redis.Nil):
		return payload, nil
	default:
		return nil, fmt.Errorf("failed to get previous payload: %v", err)
	}

	payloadPrev, err := UssdPayloadFromJSON([]byte(v))
	if err != nil {
		return nil, fmt.Errorf("failed to get json payload: %v", err)
	}

	return payloadPrev, nil
}

// PreviousMenuInvalid will return render the previous menu content prefixed by the error string
//
// This helper is usually called after the user has input wrong details and you want to return the same menu
// but a the helper string appended on the top of the menu.
// The helper is mearnt to guide the user on what went wrong
func (app *UssdApp) PreviousMenuInvalid(ctx context.Context, payload UssdPayload, helper string) (*SessionResponse, error) {
	payloadPrev, err := app.GetPreviousPayload(ctx, payload)
	if err != nil {
		return nil, err
	}

	currMenu, err := app.GetCurrentMenu(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to get current menu: %v", err)
	}

	sessionResponse, err := currMenu.GenerateMenuFn(ctx, payloadPrev, currMenu)
	if err != nil {
		return nil, err
	}

	sessionResponse.MenuName = currMenu.MenuName()
	sessionResponse.Failed = true
	sessionResponse.StatusMessage = helper

	return sessionResponse, nil
}

// GetShortCutMenu is a helper to find the first menu registered with the given shortcut
//
// A shortcut in this case is the ussd string data that comes during first session
//
// The method should only be called for new sessions as ongoing session cannot be deemed as shortcut
func (app *UssdApp) GetShortCutMenu(ctx context.Context, payload UssdPayload) string {
	shortCut := payload.UssdParams()

	if shortCut == "" {
		return ""
	}

	for _, v := range app.allmenus {
		if v.ShortCut() == shortCut {
			return v.MenuName()
		}
	}

	return ""
}
