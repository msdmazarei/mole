package packets

import (
	"crypto/md5"
	"crypto/rand"
	"fmt"
	mrand "math/rand"
	"time"

	"github.com/sirupsen/logrus"
)

type AuthRequestPacket struct {
	randomStringLength byte
	hashedStringLength byte
	usernameLength     byte
	RandomString       string
	HashedString       string
	Username           string
}

func generateHashedString(randomString, secret string) string {
	combined := fmt.Sprintf("%s%s%s", randomString, secret, randomString)
	hasher := md5.New()
	hasher.Write([]byte(combined))
	return string(hasher.Sum(nil))
}

func randomString(maxLength int) string {
	buf := make([]byte, maxLength+1)
	_, err := rand.Read(buf)
	if err != nil {
		logrus.Error("error in generating random string:", err)
	}
	return string(buf)
}

func NewAuhRequestPacket(username, secret string) AuthRequestPacket {
	const (
		maxRandomLength = 50
		minRandomLength = 1
	)

	mrand.Seed(time.Now().Unix())
	rndString := randomString(maxRandomLength + minRandomLength)
	hashedString := generateHashedString(rndString, secret)
	rtn := AuthRequestPacket{
		randomStringLength: byte(len(rndString)),
		hashedStringLength: byte(len(hashedString)),
		usernameLength:     byte(len(username)),
		RandomString:       rndString,
		HashedString:       hashedString,
		Username:           username,
	}

	return rtn
}

func (*AuthRequestPacket) dedicatedFunctionForMolePackets() {}

func (a *AuthRequestPacket) WriteTo(buf []byte) error {
	if uint16(len(buf)) < a.TotalLength() {
		return ErrTooShort
	}
	i := 0
	buf[i] = a.randomStringLength
	i++
	buf[i] = a.hashedStringLength
	i++
	buf[i] = a.usernameLength
	i++
	copy(buf[i:], []byte(a.RandomString))
	i += len(a.RandomString)
	copy(buf[i:], []byte(a.HashedString))
	i += len(a.HashedString)
	copy(buf[i:], []byte(a.Username))
	return nil
}

func (a *AuthRequestPacket) FromBytes(buf []byte) error {
	l := len(buf)
	if l < 4 || l < int(buf[0])+int(buf[1])+int(buf[2])+3 {
		return ErrTooShort
	}
	a.randomStringLength = buf[0]
	a.hashedStringLength = buf[1]
	a.usernameLength = buf[2]
	i := 3
	a.RandomString = string(buf[i : i+int(a.randomStringLength)])
	i += int(a.randomStringLength)
	a.HashedString = string(buf[i : i+int(a.hashedStringLength)])
	i += int(a.hashedStringLength)
	a.Username = string(buf[i : i+int(a.usernameLength)])
	return nil
}

func (a *AuthRequestPacket) TotalLength() uint16 {
	return 3 + uint16(a.hashedStringLength) + uint16(a.randomStringLength) + uint16(a.usernameLength)
}

func (*AuthRequestPacket) GetPacketType() PacketType {
	return AuthRequestType
}

func (a *AuthRequestPacket) Authenticate(secret string) bool {
	return generateHashedString(a.RandomString, secret) == a.HashedString
}
