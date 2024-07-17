package main

import (
	"context"

	"github.com/gidyon/ussdapp"
)

const (
	// swahili = "SWAHILI"
	english = "ENGLISH"

	homeUnregisteredMenu  = "HOME_UNREGISTERED_MENU"
	namesMenu             = "REGISTER_NAMES_MENU"
	successMenu           = "REGISTER_SUCCESS_MENU"
	loginMenu             = "LOGIN_MENU"
	loginHomeMenu         = "LOGIN_HOME_MENU"
	viewRegistrationsMenu = "VIEW_REGS_MENU"
)

var (
	users = map[string]string{}
)

func registerUserMenus(ussdApp *ussdapp.UssdApp) {
	// Home menu
	handleErr(ussdApp.AddMenu(ussdapp.NewMenu(&ussdapp.MenuOptions{
		MenuName:     homeUnregisteredMenu,
		PreviousMenu: homeUnregisteredMenu,
		NextMenu:     namesMenu,
		MenuContent: map[string]string{
			english: "Welcome to the Registration App. \n1. Register\n 2. Login",
		},
		GenerateMenuFn: func(ctx context.Context, up ussdapp.UssdPayload, menu ussdapp.Menu) (ussdapp.SessionResponse, error) {
			return menu.ExecuteMenuArgs(english), nil
		},
	})))

	// This is next menu for homeUnregisteredMenu
	// Names menu
	handleErr(ussdApp.AddMenu(ussdapp.NewMenu(&ussdapp.MenuOptions{
		MenuName:     namesMenu,
		PreviousMenu: homeUnregisteredMenu,
		NextMenu:     successMenu,
		MenuContent: map[string]string{
			english: "What is your names. \n\n0. Back 00. Main Menu",
		},
		GenerateMenuFn: func(ctx context.Context, up ussdapp.UssdPayload, menu ussdapp.Menu) (ussdapp.SessionResponse, error) {
			// Since it's the default next menu for homeUnregisteredMenu,
			// we check if user selected 2 which means will replace it with Login menu
			switch up.UssdCurrentParam() {
			case "1":
				// We do nothing and let this menu get executed at return below
			case "2":
				// We replace with Login menu
				return ussdApp.ReplaceMenuWithName(ctx, loginMenu, up)
			default:
				// They did not select 1 or 2
				return ussdApp.PreviousMenuWithError(ctx, up, menu, "Wrong selection")
			}

			return menu.ExecuteMenuArgs(english), nil
		},
	})))

	// This is next menu for namesMenu
	// Success menu
	handleErr(ussdApp.AddMenu(ussdapp.NewMenu(&ussdapp.MenuOptions{
		MenuName:     successMenu,
		PreviousMenu: namesMenu,
		NextMenu:     homeUnregisteredMenu,
		MenuContent: map[string]string{
			english: "END You have registred successfully!",
		},
		GenerateMenuFn: func(ctx context.Context, up ussdapp.UssdPayload, menu ussdapp.Menu) (ussdapp.SessionResponse, error) {
			// Save user
			users[up.UssdCurrentParam()] = up.UssdCurrentParam()

			// Return menu message
			return menu.ExecuteMenuArgs(english), nil
		},
	})))

	// User selected 2
	// Login menu
	handleErr(ussdApp.AddMenu(ussdapp.NewMenu(&ussdapp.MenuOptions{
		MenuName:     loginMenu,
		PreviousMenu: homeUnregisteredMenu,
		NextMenu:     loginHomeMenu,
		MenuContent: map[string]string{
			english: "To continue enter your username. \n\n00. Main Menu",
		},
		GenerateMenuFn: func(ctx context.Context, up ussdapp.UssdPayload, menu ussdapp.Menu) (ussdapp.SessionResponse, error) {
			// Since it's the default next menu, we check if user selected 2 which means will replace it with Login menu
			switch up.UssdCurrentParam() {
			case "00":
				// We replace with Home menu
				return ussdApp.ReplaceMenuWithName(ctx, homeUnregisteredMenu, up)
			}

			// Check if user exists
			_, ok := users[up.UssdCurrentParam()]
			if !ok {
				return ussdApp.PreviousMenuWithError(ctx, up, menu, "Username does not exist")
			}

			// We can save data to cache
			// err := ussdApp.Cache().SetMap(ctx, ussdApp.GetSessionKey(up), map[string]interface{}{
			// 	"names": up.UssdCurrentParam(),
			// })
			// if err != nil {
			// 	return nil, err
			// }

			// Execute menu, will navigate to next menu and pass the param
			return menu.ExecuteMenuArgs(english, up.UssdCurrentParam()), nil
		},
	})))

	// Login Home
	handleErr(ussdApp.AddMenu(ussdapp.NewMenu(&ussdapp.MenuOptions{
		MenuName:     loginHomeMenu,
		PreviousMenu: loginMenu,
		NextMenu:     viewRegistrationsMenu,
		MenuContent: map[string]string{
			english: "Welcome %s. \n\n1. View Registrations\n2. New Ticket\n3. My Account\n4. Change Language",
		},
		GenerateMenuFn: func(ctx context.Context, up ussdapp.UssdPayload, menu ussdapp.Menu) (ussdapp.SessionResponse, error) {
			// Execute menu, will navigate to next menu which is viewRegistrationsMenu
			return menu.ExecuteMenuArgs(english), nil
		},
	})))

	// This is next menu for loginHomeMenu
	handleErr(ussdApp.AddMenu(ussdapp.NewMenu(&ussdapp.MenuOptions{
		MenuName:     viewRegistrationsMenu,
		PreviousMenu: loginHomeMenu,
		NextMenu:     "",
		MenuContent: map[string]string{
			english: "You have no registrations",
		},
		GenerateMenuFn: func(ctx context.Context, up ussdapp.UssdPayload, menu ussdapp.Menu) (ussdapp.SessionResponse, error) {
			// user may select 2, 3, 4, or sth arbitrary
			switch up.UssdCurrentParam() {
			case "1":

			case "2":
				// We replace with New Ticket menu
				// return ussdApp.ReplaceMenuWithName(ctx, "ticket_menu", up)
			case "3":
				// We replace with My Account menu
				// return ussdApp.ReplaceMenuWithName(ctx, "my_account", up)
			case "4":
				// We replace with Change language menu
				// return ussdApp.ReplaceMenuWithName(ctx, "language", up)
			default:
				return ussdApp.PreviousMenuWithError(ctx, up, menu, "Incorrect selection")
			}

			// default menu
			return menu.ExecuteMenuArgs(english), nil
		},
	})))
}
