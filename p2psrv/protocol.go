package p2psrv

// Protocol represents a P2P subprotocol implementation.
type Protocol struct {
	// Name should contain the official protocol name,
	// often a three-letter word.
	Name string

	// Version should contain the version number of the protocol.
	Version uint32

	// Length should contain the number of message codes used
	// by the protocol.
	Length uint64

	HandleRequest HandleRequest
}
