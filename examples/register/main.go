package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/gidyon/gomicro/pkg/conn"
	"github.com/gidyon/gomicro/pkg/grpc/zaplogger"
	"github.com/gidyon/micro/utils/errs"
	"github.com/gidyon/ussdapp"
	rediscache "github.com/gidyon/ussdapp/cache/redis"
	"github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	configFile = flag.String("config-file", ".env", "Configuration file")
)

func main() {
	flag.Parse()

	ctx := context.Background()

	// Config in .env
	viper.SetConfigFile(*configFile)

	// Read config from .env
	err := viper.ReadInConfig()
	errs.Panic(err)

	// Initialize logger
	errs.Panic(zaplogger.Init(viper.GetInt("logLevel"), ""))

	zaplogger.Log = zaplogger.Log.WithOptions(zap.WithCaller(true))

	// gRPC logger compatible
	appLogger := zaplogger.ZapGrpcLoggerV2(zaplogger.Log)

	// Open gorm connection
	sqlDB, err := conn.OpenGorm(&conn.DbOptions{
		Name:     viper.GetString("mysqlName"),
		Dialect:  viper.GetString("mysqlDialect"),
		Address:  viper.GetString("mysqlAddress"),
		User:     viper.GetString("mysqlUser"),
		Password: viper.GetString("mysqlPassword"),
		Schema:   viper.GetString("mysqlSchema"),
	})
	errs.Panic(err)

	sqlDB = sqlDB.Debug()

	// Open redis connection
	redisDB := redis.NewClient(&redis.Options{
		Network:  viper.GetString("redisNetwork"),
		Addr:     viper.GetString("redisAddress"),
		Username: viper.GetString("redisUsername"),
		Password: viper.GetString("redisPassword"),
	})

	// Ussd App Instance
	ussdApp, err := ussdapp.NewUssdApp(ctx, &ussdapp.Options{
		AppName:         "test",
		HomeMenu:        homeUnregisteredMenu,
		SQLDB:           sqlDB,
		Cache:           rediscache.NewRedisCache(redisDB),
		Logger:          appLogger,
		TableName:       viper.GetString("LOGS_TABLE"),
		DefaultLanguage: english,
		SaveLogs:        false,
		Handler:         nil,
		SessionDuration: 3 * time.Minute,
	})
	handleErr(err)

	// Register menus
	registerUserMenus(ussdApp)

	http.HandleFunc("/ussd", ussdHandler(ussdApp))
	handleErr(http.ListenAndServe(viper.GetString("httpPort"), nil))
}

func handleErr(err error) {
	if err != nil {
		panic(err)
	}
}

func ussdHandler(ussdApp *ussdapp.UssdApp) http.HandlerFunc {
	handler := func(rw http.ResponseWriter, r *http.Request) (int, error) {
		bs, err := httputil.DumpRequest(r, true)
		if err != nil {
			return http.StatusInternalServerError, err
		}

		log.Printf("INCOMING REQUEST: %s\n\n", string(bs))

		var (
			ctx           = r.Context()
			payload       = ussdapp.UssdPayloadFromRequest(r)
			sessionResp   ussdapp.SessionResponse
			sessionFailed = func(status string) {
				ussdapp.SetSessionFailed(sessionResp, status)
			}
			currentMenu ussdapp.Menu
		)

		// Save session response at end of function
		defer func() {
			if sessionResp != nil {
				ussdApp.SaveLog(ctx, payload, sessionResp)
			}
		}()

		// Check if user is registered
		exists, err := true, nil

		// Check if session is new
		currentMenu, newSession, err := ussdApp.GetSessionMenu(ctx, payload)
		if err != nil {
			sessionFailed(err.Error())
			return http.StatusInternalServerError, err
		}

		_, _ = newSession, exists

		// switch {
		// case exists && newSession:
		// 	// Save user details to cache
		// 	err = ussdApp.Cache().SetMap(ctx, ussdApp.GetSessionKey(payload), map[string]interface{}{
		// 		"full_names": "Test User",
		// 	})
		// 	if err != nil {
		// 		return http.StatusInternalServerError, fmt.Errorf("failed to save user to cache: %v", err)
		// 	}

		// 	// Update current menu in this case
		// 	currentMenu, err = ussdApp.SaveMenuNameAsCurrent(ctx, loginMenu, payload)
		// 	if err != nil {
		// 		return http.StatusInternalServerError, fmt.Errorf("failed to save %s menu as current: %v", loginMenu, err)
		// 	}
		// }

		// Generate response to send to user
		sessionResp, err = currentMenu.GenerateResponse(ctx, payload)
		switch {
		case err == nil:
			// Write response back to user: IMPORTANT
			err = ussdapp.WriteUSSDResponse(rw, payload, sessionResp)
			if err != nil {
				return http.StatusInternalServerError, err
			}

		case errors.Is(err, ussdapp.ErrFailedValidation):
			sessionFailed(err.Error())
		default:
			sessionFailed(err.Error())
			return http.StatusInternalServerError, err
		}

		return http.StatusOK, nil
	}

	return func(rw http.ResponseWriter, r *http.Request) {
		c, err := handler(rw, r)
		if err != nil {
			log.Printf("ussd request failed with code %v %v", c, err)
			http.Error(rw, "END Service is not available try again later", http.StatusOK)
		}
	}
}
