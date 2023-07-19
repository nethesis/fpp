package main

/** Structs definition **/

type Response struct {
	Message string `json:"message"`
}

type Notification struct {
	CallId      string `json:"call-id"`
	Uuid        string `json:"uuid"`
	Topic       string `json:"topic"`
	Type        string `json:"type"`
	FromUri     string `json:"from-uri"`
	DisplayName string `json:"display-name"`
}

type Registration struct {
	Token string `json:"token"`
	Topic string `json:"topic"`
}
