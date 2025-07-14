package wsclient

// EventWrapper is used to return errors from the websocket alongside the data
type EventWrapper[T any] struct {
	Data  T
	Error error
}

// Subscription is used to handle the active subscription
type Subscription[T any] struct {
	EventChan   <-chan EventWrapper[T]
	Unsubscribe func() error
}
