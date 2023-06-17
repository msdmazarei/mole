package packets

type RejectCode byte

const (
	RejectCodeUnknown RejectCode = iota
	RejectCodeBadSecret
	RejectCodeServiceUnavailable
)

type AuthRejectPacket = StatusMessagePacket[RejectCode]

func NewAuthRejectPacket(code RejectCode, msg string) AuthRejectPacket {
	return NewStatusMessage[RejectCode](AuthRejectType, code, msg)
}
