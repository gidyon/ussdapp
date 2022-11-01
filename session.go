package ussdapp

// SessionResponse is response for ussd session request
type SessionResponse interface {
	// Response return the session response string
	Response() string
	// Failed indicates if the USSD request failed due to an API error, incorrect input, etc
	Failed() bool
	// StatusMessage for the USSD session
	StatusMessage() string
	// MenuName returns the name of the USSD menu executing current request
	MenuName() string
	// SessionId returns the USSD session id
	SessionId() string

	// unexposed setters
	setResponse(string)
	setFailed()
	setStatusMessage(string)
	setMenu(string)
	setSessionId(string)
}

type sessionResponse struct {
	response      string
	failed        bool
	statusMessage string
	menuName      string
	sessionId     string
}

func (sr *sessionResponse) Response() string {
	return sr.response
}

func (sr *sessionResponse) Failed() bool {
	return sr.failed
}

func (sr *sessionResponse) StatusMessage() string {
	return sr.statusMessage
}

func (sr *sessionResponse) MenuName() string {
	return sr.menuName
}

func (sr *sessionResponse) SessionId() string {
	return sr.sessionId
}

func (sr *sessionResponse) setResponse(val string) {
	sr.response = val
}

func (sr *sessionResponse) setFailed() {
	sr.failed = true
}

func (sr *sessionResponse) setMenu(val string) {
	sr.menuName = val
}

func (sr *sessionResponse) setStatusMessage(val string) {
	sr.statusMessage = val
}

func (sr *sessionResponse) setSessionId(val string) {
	sr.sessionId = val
}

type SessionData struct {
	Response      string
	Failed        bool
	StatusMessage string
	MenuName      string
	SessionId     string
}

func NewSessionResponse(data *SessionData) SessionResponse {
	return &sessionResponse{
		response:      data.Response,
		failed:        data.Failed,
		statusMessage: data.StatusMessage,
		menuName:      data.MenuName,
		sessionId:     data.SessionId,
	}
}

func SetSessionFailed(session SessionResponse, status string) {
	if session == nil {
		session = &sessionResponse{}
	}
	session.setFailed()
	session.setStatusMessage(status)
}
