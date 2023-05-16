package packets

import (
	"crypto/md5"
	"crypto/rand"
	"fmt"
	mrand "math/rand"
	"time"
)

type AuthRequestPacket struct {
	randomStringLength byte
	hashedStringLength byte
	RandomString       string
	HashedString       string
}

func generate_hashed_string(random_string, secret string) string {
	combined := fmt.Sprintf("%s%s%s", random_string, secret, random_string)
	hasher := md5.New()
	hasher.Write([]byte(combined))
	return string(hasher.Sum(nil))
}

func random_string(max_length int) string {
	buf := make([]byte, max_length+1)
	rand.Read(buf)
	return string(buf)
}

func NewAuhRequestPacket(secret string) AuthRequestPacket {
	mrand.Seed(time.Now().Unix())
	rnd_string := random_string(mrand.Intn(49) + 1)
	hashed_string := generate_hashed_string(rnd_string, secret)
	rtn := AuthRequestPacket{
		randomStringLength: byte(len(rnd_string)),
		hashedStringLength: byte(len(hashed_string)),
		RandomString:       rnd_string,
		HashedString:       hashed_string,
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
	return generate_hashed_string(a.RandomString, secret) == a.HashedString
}
