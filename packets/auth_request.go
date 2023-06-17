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
	RandomString       string
	HashedString       string
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

func NewAuhRequestPacket(secret string) AuthRequestPacket {
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
		RandomString:       rndString,
		HashedString:       hashedString,
	}

	return rtn
}

func (*AuthRequestPacket) dedicatedFunctionForMolePackets() {}

func (a *AuthRequestPacket) WriteTo(buf []byte) error {
	if uint16(len(buf)) < a.TotalLength() {
		return ErrTooShort
	}
	buf[0] = a.randomStringLength
	buf[1] = a.hashedStringLength
	copy(buf[2:], []byte(a.RandomString))
	copy(buf[2+buf[0]:], []byte(a.HashedString))
	return nil
}

func (a *AuthRequestPacket) FromBytes(buf []byte) error {
	l := len(buf)
	if l < 4 || l < int(buf[0])+int(buf[1])+2 {
		return ErrTooShort
	}
	a.randomStringLength = buf[0]
	a.hashedStringLength = buf[1]
	i := 2
	a.RandomString = string(buf[i : i+int(a.randomStringLength)])
	i += int(a.randomStringLength)
	a.HashedString = string(buf[i : i+int(a.hashedStringLength)])
	return nil
}

func (a *AuthRequestPacket) TotalLength() uint16 {
	return 2 + uint16(a.hashedStringLength) + uint16(a.randomStringLength)
}

func (*AuthRequestPacket) GetPacketType() PacketType {
	return AuthRequestType
}

func (a *AuthRequestPacket) Authenticate(secret string) bool {
	return generateHashedString(a.RandomString, secret) == a.HashedString
}
