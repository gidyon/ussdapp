package ussdapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"google.golang.org/grpc/grpclog"
	"gorm.io/gorm"
)

const (
	currentMenuKey  = "current_menu"
	previousMenuKey = "previous_menu"
	currentPayload  = "current_payload"
	languageKey     = "language"
)

type UssdApp struct {
	homeMenu string
	allmenus map[string]Menu
	menus    []string
	logsChan chan *SessionRequest
	opt      *Options
}

// Options contains data required for ussd app
type Options struct {
	AppName         string
	HomeMenu        string
	SQLDB           *gorm.DB
	Cache           Cacher
	Logger          grpclog.LoggerV2
	TableName       string
	DefaultLanguage string
	SaveLogs        bool
	Handler         http.Handler
	SessionDuration time.Duration
}

// NewUssdApp returns a ussd application to be configured
func NewUssdApp(ctx context.Context, opt *Options) (*UssdApp, error) {
	// Validation
	switch {
	case opt == nil:
		return nil, errors.New("missing options")
	case opt.AppName == "":
		return nil, errors.New("missing app name")
	case opt.HomeMenu == "":
		return nil, errors.New("missing home menu")
	case opt.Cache == nil:
		return nil, errors.New("missing redis db")
	case opt.SQLDB == nil:
		return nil, errors.New("missing sql db")
	case opt.Logger == nil:
		return nil, errors.New("missing logger")
	default:
		if opt.SessionDuration == 0 {
			opt.SessionDuration = time.Minute * 10
		}
	}

	if opt.TableName != "" {
		sessionLogsTable = opt.TableName
	} else {
		sessionLogsTable = os.Getenv("USSD_LOGS_TABLE")
	}

	app := &UssdApp{
		homeMenu: opt.HomeMenu,
		allmenus: make(map[string]Menu),
		menus:    []string{},
		logsChan: make(chan *SessionRequest, bulkInsertSize),
		opt:      opt,
	}

	// Auto migration
	if !app.opt.SQLDB.Migrator().HasTable(&SessionRequest{}) {
		err := app.opt.SQLDB.AutoMigrate(&SessionRequest{})
		if err != nil {
			return nil, fmt.Errorf("failed to auto migrate %s table", (&SessionRequest{}).TableName())
		}
	}

	if opt.SaveLogs {
		// Start insert worker
		go app.saveLogsWorker(ctx)

		// Start failed insert worker
		go app.saveFailedLogsWorker(ctx)
	}

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
	case m.NextMenu() == "":
		return fmt.Errorf("next menu for %s is missing", m.MenuName())
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

	app.opt.Logger.Infof("Registered %s menu", m.MenuName())

	return nil
}

func (app *UssdApp) Handler() http.Handler {
	return app.opt.Handler
}

func (app *UssdApp) Cache() Cacher {
	return app.opt.Cache
}

func (app *UssdApp) Logger() grpclog.LoggerV2 {
	return app.opt.Logger
}

func (app *UssdApp) SqlDB() *gorm.DB {
	return app.opt.SQLDB
}

func (app *UssdApp) sessionKey(payload UssdPayload) string {
	return fmt.Sprintf("%s:sessions:%s:%s", app.opt.AppName, payload.SessionId(), payload.Msisdn())
}

// GetMenuNames will return all menu names registered as a slice of strings
func (app *UssdApp) GetMenuNames() []string {
	return app.menus
}

// GetNextMenu will attempt to get the highest matching menu to be saved or/and rendered
func (app *UssdApp) GetNextMenu(currentMenu Menu, payload UssdPayload) (Menu, error) {
	next, ok := app.allmenus[currentMenu.NextMenu()]
	if !ok {
		return nil, fmt.Errorf("%v: %s", ErrMenuNotExist, currentMenu.NextMenu())
	}

	return next, nil
}

// SaveMenuNameAsCurrent will save the menu eith given name as current.
func (app *UssdApp) SaveMenuNameAsCurrent(ctx context.Context, menuName string, payload UssdPayload) (Menu, error) {
	menu, ok := app.allmenus[menuName]
	if !ok {
		return nil, ErrMenuNotExist
	}

	err := app.SaveMenuAsCurrent(ctx, menu, payload)
	if err != nil {
		return nil, err
	}

	return menu, nil
}

// SaveMenuAsCurrent will save the menu as current in cache.
//
// It will be used for the next incoming request to determine the right menu to render
//
// Will fail of the menu does not exist
func (app *UssdApp) SaveMenuAsCurrent(ctx context.Context, menu Menu, payload UssdPayload) error {
	bs, err := payload.JSON()
	if err != nil {
		return err
	}

	key := app.sessionKey(payload)
	err = app.opt.Cache.SetMap(
		ctx,
		key,
		map[string]interface{}{
			currentMenuKey:  menu.MenuName(),
			previousMenuKey: menu.PreviousMenu(),
			currentPayload:  bs,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to set current_menu and payload to map: %v", err)
	}

	return nil
}

// GetCurrentMenu will get the current menu
//
// If no menu is found, it will default to home menu
//
// To set the home menu, use the helper SetHomeMenu
func (app *UssdApp) GetCurrentMenu(ctx context.Context, payload UssdPayload) (Menu, error) {
	res, err := app.opt.Cache.GetMapField(ctx, app.sessionKey(payload), currentMenuKey)
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

func sessionSetKey(appId string) string {
	return fmt.Sprintf("%s:sessions", appId)
}

// IsNewSession will check if incoming ussd session is new
//
// If session is new, it will be saved and automatically be cleared after session duration
func (app *UssdApp) IsNewSession(ctx context.Context, payload UssdPayload) (bool, error) {
	exists, err := app.opt.Cache.SetUnique(ctx, sessionSetKey(app.opt.AppName), fmt.Sprintf("%s:%s", payload.Msisdn(), payload.SessionId()))
	if err != nil {
		return false, fmt.Errorf("failed to check if session is new: %v", err)
	}

	var (
		sessionID = payload.SessionId()
		msisdn    = payload.Msisdn()
	)

	if !exists {
		time.AfterFunc(app.opt.SessionDuration, func() {
			// Delete user cache
			key := app.sessionKey(payload)
			err = app.Cache().Delete(context.Background(), key)
			if err != nil {
				app.Logger().Errorf("ERROR DELETING SESSION DATA: %v", err)
			}
			// Remove session key from set
			err := app.deleteSessionSetKey(context.Background(), msisdn, sessionID)
			if err != nil {
				app.Logger().Errorf("ERROR DELETING SESSION: %v", err)
				return
			}
			app.Logger().Infof("SESSION %s DELETED", sessionID)
		})
	}

	return !exists || payload.UssdParams() == "", nil
}

// deleteSessionSetKey will remove session key from cache
func (app *UssdApp) deleteSessionSetKey(ctx context.Context, sessionID, msisdn string) error {
	err := app.opt.Cache.DeleteSetValue(ctx, sessionSetKey(app.opt.AppName), fmt.Sprintf("%s:%s", msisdn, sessionID))
	if err != nil {
		return fmt.Errorf("failed to remove session: %v", err)
	}
	return nil
}

// SaveLanguage will save user language for the ussd session
func (app *UssdApp) SaveLanguage(ctx context.Context, payload UssdPayload, language string) error {
	err := app.opt.Cache.SetMapField(ctx, app.sessionKey(payload), languageKey, language)
	if err != nil {
		return fmt.Errorf("failed to save language")
	}
	return nil
}

// GetLanguage will get the preferred language for the ussd session
func (app *UssdApp) GetLanguage(ctx context.Context, payload UssdPayload) string {
	lang, err := app.opt.Cache.GetMapField(ctx, app.sessionKey(payload), languageKey)
	if err != nil {
		return app.opt.DefaultLanguage
	}
	return lang
}

// GetSessionKey will get the session key that is used to save the user data in cache
func (app *UssdApp) GetSessionKey(payload UssdPayload) string {
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

func (app *UssdApp) ReplaceMenuWithName(ctx context.Context, menuName string, payload UssdPayload) (SessionResponse, error) {
	menu, ok := app.allmenus[menuName]
	if !ok {
		return nil, ErrMenuNotExist
	}

	return app.ReplaceMenu(ctx, payload, menu)
}

func (app *UssdApp) ReplaceMenu(ctx context.Context, payload UssdPayload, menu Menu) (SessionResponse, error) {
	err := app.SaveMenuAsCurrent(ctx, menu, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to save current menu: %v", err)
	}

	sr, err := menu.GenerateResponse(ctx, payload)
	if err != nil {
		return nil, err
	}

	err = app.UpdateNextMenu(ctx, payload, menu)
	if err != nil {
		return nil, err
	}

	payload.(*ussdPayload).data.skip = true
	sr.setMenu(menu.MenuName())

	return sr, nil
}

// getPreviousPayload will get the payload for the ussd request
func (app *UssdApp) getPreviousPayload(ctx context.Context, payload UssdPayload) (UssdPayload, error) {
	v, err := app.opt.Cache.GetMapField(ctx, app.sessionKey(payload), currentPayload)
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
func (app *UssdApp) PreviousMenuInvalid(ctx context.Context, payload UssdPayload, currMenu Menu, helper string) (SessionResponse, error) {
	payloadPrev, err := app.getPreviousPayload(ctx, payload)
	if err != nil {
		return nil, err
	}

	fmt.Println("Previous payload: ", payloadPrev.UssdCurrentParam())

	prevMenu, ok := app.allmenus[currMenu.PreviousMenu()]
	if !ok {
		return nil, fmt.Errorf("previous menu does not exist %s: %w", currMenu.PreviousMenu(), ErrMenuExist)
	}

	sr, err := prevMenu.GenerateResponse(ctx, payloadPrev)
	if err != nil {
		return nil, err
	}

	sr.setMenu(prevMenu.MenuName())
	payload.(*ussdPayload).data.ValidationFailed = true
	sr.setFailed()
	sr.setStatusMessage(helper)

	return sr, nil
}

// GetShortCutMenu is a helper to find the first menu registered with the given shortcut
//
// A shortcut in this case is the ussd string data that comes during first session
//
// The method should only be called for new sessions as ongoing session cannot be deemed as shortcut
func (app *UssdApp) GetShortCutMenu(ctx context.Context, payload UssdPayload) Menu {
	shortCut := payload.UssdParams()

	if shortCut == "" {
		return nil
	}

	for _, v := range app.allmenus {
		if v.ShortCut() == shortCut {
			return v
		}
	}

	return nil
}

const (
	conPrefix = "CON"
	endPrefix = "END"
	uprPrefix = "UPR"
)

func WriteUSSDResponse(w http.ResponseWriter, up UssdPayload, sr SessionResponse) error {
	res := strings.TrimSpace(sr.Response())

	if sr.Failed() || up.ValidationFailed() {
		valErr := sr.StatusMessage()
		switch {
		case strings.HasPrefix(res, conPrefix):
			res = fmt.Sprintf("CON %s\n%s", valErr, res[4:])
		case strings.HasPrefix(res, endPrefix):
			res = fmt.Sprintf("END %s\n%s", valErr, res[4:])
		case strings.HasPrefix(res, uprPrefix):
		default:
			res = fmt.Sprintf("CON %s\n%s", valErr, res)
		}
	}

	if !strings.HasPrefix(res, "CON ") && !strings.HasPrefix(res, "UPR") && !strings.HasPrefix(res, "END") {
		res = fmt.Sprintf("CON %s", res)
	}

	_, err := io.WriteString(w, res)
	if err != nil {
		return err
	}

	return nil
}

// UpdateNextMenu will get the next menu for current menu and save it as current menu
func (app *UssdApp) UpdateNextMenu(ctx context.Context, payload UssdPayload, currMenu Menu) error {
	if payload.ValidationFailed() || payload.(*ussdPayload).data.skip {
		return nil
	}

	// Get next menu
	m, err := app.GetNextMenu(currMenu, payload)
	if err != nil {
		return err
	}

	// Save menu as current
	err = app.SaveMenuAsCurrent(ctx, m, payload)
	if err != nil {
		return err
	}

	return nil
}

func failedStatus(failed ...bool) bool {
	for _, fail := range failed {
		if fail {
			return true
		}
	}
	return false
}

// SaveLog will save ussd log to database for audit or traceback purposes
//
// If saving logs is disabled, the method has no effect
func (app *UssdApp) SaveLog(ctx context.Context, payload UssdPayload, sr SessionResponse) {
	if !app.opt.SaveLogs {
		return
	}

	if sr == nil {
		sr = &sessionResponse{}
	}

	t := time.Now()

	select {
	case <-ctx.Done():
	case app.logsChan <- &SessionRequest{
		SessionID:     payload.SessionId(),
		Msisdn:        payload.Msisdn(),
		USSDParams:    payload.UssdParams(),
		UserInput:     payload.UssdCurrentParam(),
		MenuName:      sr.MenuName(),
		Succeeded:     !failedStatus(sr.Failed(), payload.ValidationFailed()),
		StatusMessage: sr.StatusMessage(),
		CreatedAt:     t,
	}:
	}
}
