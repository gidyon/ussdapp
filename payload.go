package ussdapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// UssdPayload interface has getters for getting original session data for the ussd request.
// It is an interface to prevents inadvertent modification of ussd data in the lifetime of the ussd request
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
	// make data unexported
	data *ussdPayloadInternal
}

// json serializable
type ussdPayloadInternal struct {
	SessionID        string `json:"session_id,omitempty"`
	ServiceCode      string `json:"service_code,omitempty"`
	Msisdn           string `json:"msisdn,omitempty"`
	UssdParams       string `json:"ussd_params,omitempty"`
	UssdCurrentParam string `json:"ussd_current_param,omitempty"`
	IsShortCut       bool   `json:"is_short_cut,omitempty"`
	ValidationFailed bool   `json:"validation_failed,omitempty"`
	Time             string `json:"time,omitempty"`
	skip             bool
}

func (p *ussdPayload) SkipSaving() bool {
	return p.data.skip
}

func (p *ussdPayload) SessionId() string {
	return p.data.SessionID
}

func (p *ussdPayload) ServiceCode() string {
	return p.data.ServiceCode
}

func (p *ussdPayload) Msisdn() string {
	return p.data.Msisdn
}

func (p *ussdPayload) UssdParams() string {
	return p.data.UssdParams
}

func (p *ussdPayload) JSON() ([]byte, error) {
	return json.Marshal(p.data)
}

func (p *ussdPayload) UssdCurrentParam() string {
	return p.data.UssdCurrentParam
}
func (p *ussdPayload) IsShortCut() bool {
	return p.data.IsShortCut
}
func (p *ussdPayload) ValidationFailed() bool {
	return p.data.ValidationFailed
}
func (p *ussdPayload) Time() string {
	return p.data.Time
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

type incomingUssd struct {
	Msisdn      string `json:"msisdn"`
	SessionID   string `json:"sessionId"`
	ServiceCode string `json:"serviceCode"`
	UssdString  string `json:"ussdString"`
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
			data: &ussdPayloadInternal{
				SessionID:        getQueryVal(params, "SESSION_ID", "session-id", "session_id", "session"),
				ServiceCode:      getQueryVal(params, "SERVICE_CODE", "ORIG", "service-code", "service_code"),
				Msisdn:           getQueryVal(params, "DEST", "MSISDN", "msisdn"),
				UssdParams:       getQueryVal(params, "USSD_PARAMS", "USSD_STRING", "ussd-string", "ussd_string"),
				UssdCurrentParam: strings.TrimSpace(ussdParams[len(ussdParams)-1]),
			},
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
			data: &ussdPayloadInternal{
				SessionID:        p.SessionID,
				ServiceCode:      p.ServiceCode,
				Msisdn:           p.Msisdn,
				UssdParams:       p.UssdString,
				UssdCurrentParam: ussdParams[len(ussdParams)-1],
				IsShortCut:       false,
				ValidationFailed: false,
				Time:             "",
			},
		}
	}

	return payload
}

// UssdPayloadFromRequest will read the json byte and return an interface for reading data
func UssdPayloadFromJSON(bs []byte) (UssdPayload, error) {

	payload := &ussdPayload{
		data: &ussdPayloadInternal{},
	}

	err := json.Unmarshal(bs, payload.data)
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func SkipSavingPayload(payload UssdPayload) {
	payload.(*ussdPayload).data.skip = true
}
