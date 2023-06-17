package packets

type AcceptCode byte

const (
	AcceptCodeUnknown AcceptCode = iota
	AcceptCodeOK
	AcceptCodeWithRedirect
)

type AuthAcceptPacket = StatusMessagePacket[AcceptCode]

func NewAuthAcceptPacket(code AcceptCode, msg string) AuthAcceptPacket {
	return NewStatusMessage[AcceptCode](AuthAcceptType, code, msg)
}
