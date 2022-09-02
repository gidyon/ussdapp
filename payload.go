package ussdapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// UssdPayload interface has getters for getting original session data for the ussd request.
// It is an interface to prevents inadvertent modification of ussd data down the request chain
type UssdPayload interface {
	SessionId() string
	ServiceCode() string
	Msisdn() string
	UssdParams() string
	UssdCurrentParam() string
	IsShortCut() bool
	ValidationFailed() bool
	Time() string
	JSON() ([]byte, error)
}

type ussdPayload struct {
	sessionID        string
	serviceCode      string
	msisdn           string
	ussdParams       string
	ussdCurrentParam string
	isShortCut       bool
	validationFailed bool
	time             string
}

type incomingUssd struct {
	Msisdn      string `json:"msisdn"`
	SessionID   string `json:"sessionId"`
	ServiceCode string `json:"serviceCode"`
	UssdString  string `json:"ussdString"`
}

func (p *ussdPayload) SessionId() string {
	return p.sessionID
}

func (p *ussdPayload) ServiceCode() string {
	return p.serviceCode
}

func (p *ussdPayload) Msisdn() string {
	return p.msisdn
}

func (p *ussdPayload) UssdParams() string {
	return p.ussdParams
}

func (p *ussdPayload) JSON() ([]byte, error) {
	return json.Marshal(p)
}

func (p *ussdPayload) UssdCurrentParam() string {
	return p.ussdCurrentParam
}
func (p *ussdPayload) IsShortCut() bool {
	return p.isShortCut
}
func (p *ussdPayload) ValidationFailed() bool {
	return p.validationFailed
}
func (p *ussdPayload) Time() string {
	return p.time
}

// gets the first value from params
func getQueryVal(params url.Values, keys ...string) string {
	for _, key := range keys {
		v := params.Get(key)
		if v != "" {
			v2, err := url.QueryUnescape(v)
			if err != nil {
				return v
			}
			return v2
		}
	}
	return ""
}

// UssdPayloadFromRequest will read request params or body and return an interface for reading data
func UssdPayloadFromRequest(r *http.Request) UssdPayload {

	payload := &ussdPayload{}

	switch r.Method {
	case http.MethodGet:
		params := r.URL.Query()

		ussdParams := strings.Split(getQueryVal(params, "USSD_PARAMS", "USSD_STRING", "ussd-string", "ussd_string"), "*")
		if len(ussdParams) == 0 {
			ussdParams = []string{""}
		}

		payload = &ussdPayload{
			sessionID:        getQueryVal(params, "SESSION_ID", "session-id", "session_id", "session"),
			serviceCode:      getQueryVal(params, "SERVICE_CODE", "ORIG", "service-code", "service_code"),
			msisdn:           getQueryVal(params, "DEST", "MSISDN", "msisdn"),
			ussdParams:       getQueryVal(params, "USSD_PARAMS", "USSD_STRING", "ussd-string", "ussd_string"),
			ussdCurrentParam: strings.TrimSpace(ussdParams[len(ussdParams)-1]),
		}
	case http.MethodPost:
		p := &incomingUssd{}
		err := json.NewDecoder(r.Body).Decode(p)
		if err != nil {
			fmt.Println(err)
		}

		ussdStr, err := url.QueryUnescape(p.UssdString)
		if err != nil {
			ussdStr = p.UssdString
		}

		ussdParams := strings.Split(ussdStr, "*")
		if len(ussdParams) == 0 {
			ussdParams = []string{""}
		}

		payload = &ussdPayload{
			sessionID:        p.SessionID,
			serviceCode:      p.ServiceCode,
			msisdn:           p.Msisdn,
			ussdParams:       p.UssdString,
			ussdCurrentParam: ussdParams[len(ussdParams)-1],
			isShortCut:       false,
			validationFailed: false,
			time:             "",
		}
	}

	return payload
}

// UssdPayloadFromRequest will read the json byte and return an interface for reading data
func UssdPayloadFromJSON(jsonData []byte) (UssdPayload, error) {

	payload := &ussdPayload{}

	err := json.Unmarshal(jsonData, payload)
	if err != nil {
		return &ussdPayload{}, err
	}

	return payload, nil
}
