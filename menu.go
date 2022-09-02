package ussdapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

var (
	ErrFailedValidation = errors.New("validation failed")
	ErrMenuNotExist     = fmt.Errorf("menu does not exist")
	ErrMenuExist        = fmt.Errorf("menu is registered")
)

// Menu is an interface to be used for reading menu options. Prevents writes to the underlying menu data
type Menu interface {
	MenuName() string
	PreviousMenu() string
	NextMenu() string
	ShortCut() string
	HasArbitraryInput() bool
	GenerateMenuFn(context.Context, UssdPayload, Menu) (*SessionResponse, error)
}

type generateMenuFn func(context.Context, UssdPayload, Menu) (*SessionResponse, error)

// MenuOptions contains data for a USSD menu.
type MenuOptions struct {
	MenuName          string
	PreviousMenu      string
	NextMenu          string
	ShortCut          string
	HasArbitraryInput bool
	MenuItems         map[string]string
	MenuContent       map[string]string
	GenerateMenuFn    generateMenuFn
}

// NewMenu will create a new menu instance
func NewMenu(opt *MenuOptions) Menu {
	m := &menu{
		menuName:          opt.MenuName,
		previousMenu:      opt.PreviousMenu,
		nextMenu:          opt.NextMenu,
		shortCut:          opt.ShortCut,
		hasArbitraryInput: opt.HasArbitraryInput,
		generateMenuFn:    opt.GenerateMenuFn,
	}

	return m
}

type menu struct {
	menuName          string
	previousMenu      string
	nextMenu          string
	shortCut          string
	hasArbitraryInput bool
	menuItems         map[string]string
	generateMenuFn    generateMenuFn
	MenuContent       map[string]string
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

func (m *menu) HasArbitraryInput() bool {
	return m.hasArbitraryInput
}

func (m *menu) MenuItems() map[string]string {
	return m.menuItems
}

func (m *menu) GenerateMenuFn(ctx context.Context, p UssdPayload, menu Menu) (*SessionResponse, error) {
	return m.generateMenuFn(ctx, p, menu)
}

// MenuText returns the menu text
func (m *menu) MenuText(lang string) string {
	return m.MenuContent[lang]
}

func (m *menu) MenuResponse(langKey string, args ...interface{}) *SessionResponse {
	res := m.MenuText(langKey)
	if len(args) > 0 {
		res = fmt.Sprintf(m.MenuText(langKey), args...)
	}
	return &SessionResponse{
		Response:      res,
		Failed:        false,
		StatusMessage: "",
		MenuName:      m.menuName,
	}
}

func (m *menu) GenerateMenu(ctx context.Context, w http.ResponseWriter, up UssdPayload) (*SessionResponse, error) {
	hopResponse, err := m.generateMenuFn(ctx, up, m)
	switch {
	case err == nil:
		hopResponse.MenuName = m.menuName
	case errors.Is(err, ErrFailedValidation):
		if hopResponse == nil {
			hopResponse = &SessionResponse{}
		}

		hopResponse.Failed = true
		hopResponse.StatusMessage = firstVal(hopResponse.StatusMessage, ErrFailedValidation.Error())
		hopResponse.MenuName = m.menuName

		m.writeResponse(w, up, hopResponse)

		return hopResponse, nil
	default:
		http.Error(w, "END Try again later", http.StatusInternalServerError)

		if hopResponse == nil {
			hopResponse = &SessionResponse{}
		}

		hopResponse.Failed = true
		hopResponse.StatusMessage = firstVal(hopResponse.StatusMessage, err.Error())
		hopResponse.MenuName = m.menuName

		return hopResponse, err
	}

	m.writeResponse(w, up, hopResponse)

	return hopResponse, nil
}

const (
	conPrefix = "CON"
	endPrefix = "END"
	uprPrefix = "UPR"
)

func (m *menu) writeResponse(w http.ResponseWriter, up UssdPayload, sessionResponse *SessionResponse) {
	res := strings.TrimSpace(sessionResponse.Response)

	if sessionResponse.Failed || up.ValidationFailed() {
		valErr := firstVal(getValidationError(up), sessionResponse.StatusMessage)
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
		log.Printf("ERROR: %v", err)
	}
}
