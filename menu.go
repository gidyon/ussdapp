package ussdapp

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrFailedValidation = errors.New("validation failed")
	ErrMenuNotExist     = fmt.Errorf("menu does not exist")
	ErrMenuExist        = fmt.Errorf("menu is registered")
)

// Menu has information about a USSD menu page. It is an interface to prevent underlying menu data from unwanted writes.
type Menu interface {
	// MenuName returns the name of the menu
	MenuName() string
	// PreviousMenu returns the previous menu for this menu
	PreviousMenu() string
	// NextMenu returns the next menu for this menu
	NextMenu() string
	// Shortcut returns the shortcut text
	ShortCut() string
	// GenerateResponse calls the underlying logic registered for the menu and will return session response.
	GenerateResponse(context.Context, UssdPayload) (SessionResponse, error)
	// ExecuteMenuArgs applies the arguments to the specified menu item with given key, returning the resulting session response.
	ExecuteMenuArgs(key string, args ...interface{}) SessionResponse
}

type generateMenuFn func(context.Context, UssdPayload) (SessionResponse, error)

// MenuOptions contains data for a USSD menu.
type MenuOptions struct {
	MenuName       string
	PreviousMenu   string
	NextMenu       string
	ShortCut       string
	MenuContent    map[string]string
	GenerateMenuFn func(context.Context, UssdPayload, Menu) (SessionResponse, error)
}

type fn1 func(context.Context, UssdPayload, Menu) (SessionResponse, error)

func wrap(in fn1, m Menu) generateMenuFn {
	return func(ctx context.Context, up UssdPayload) (SessionResponse, error) {
		return in(ctx, up, m)
	}
}

// NewMenu will create a new menu instance
func NewMenu(opt *MenuOptions) Menu {
	m := &menu{
		menuName:     opt.MenuName,
		previousMenu: opt.PreviousMenu,
		nextMenu:     opt.NextMenu,
		shortCut:     opt.ShortCut,
		menuContent:  make(map[string]string, len(opt.MenuContent)),
	}
	data := make(map[string]string, len(opt.MenuContent))
	for k, v := range opt.MenuContent {
		data[k] = v
	}
	m.menuContent = data
	m.generateMenuFn = wrap(opt.GenerateMenuFn, m)

	return m
}

type menu struct {
	menuName       string
	previousMenu   string
	nextMenu       string
	shortCut       string
	generateMenuFn func(context.Context, UssdPayload) (SessionResponse, error)
	menuContent    map[string]string
}

func (m *menu) MenuName() string {
	return m.menuName
}

func (m *menu) PreviousMenu() string {
	return m.previousMenu
}

func (m *menu) NextMenu() string {
	return m.nextMenu
}

func (m *menu) ShortCut() string {
	return m.shortCut
}

func (m *menu) GenerateResponse(ctx context.Context, p UssdPayload) (SessionResponse, error) {
	res, err := m.generateMenuFn(ctx, p)
	switch {
	case err == nil:
	case errors.Is(err, ErrFailedValidation):
		if res == nil {
			res = &sessionResponse{}
		}
		res.setFailed()
		res.setStatusMessage(firstVal(res.StatusMessage(), ErrFailedValidation.Error()))
		res.setMenu(m.menuName)
	default:
		return nil, err
	}

	return res, nil
}

// menuText returns the menu text
func (m *menu) menuText(lang string) string {
	return m.menuContent[lang]
}

func (m *menu) ExecuteMenuArgs(key string, args ...interface{}) SessionResponse {
	res := m.menuText(key)
	if len(args) > 0 {
		res = fmt.Sprintf(res, args...)
	}
	return &sessionResponse{
		response:      res,
		failed:        false,
		statusMessage: "",
		menuName:      m.menuName,
	}
}
