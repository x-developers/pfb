package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"github.com/mr-tron/base58"
)

// !!! Подставьте ваш discriminator !!!
const CREATE_DISCRIMINATOR uint64 = 8530921459188068891

type CreateInstruction struct {
	Name         string
	Symbol       string
	URI          string
	Mint         string
	BondingCurve string
	User         string
	Creator      string
}

func decodeDataString(dataStr string) ([]byte, error) {
	dataStr = strings.TrimSpace(dataStr)
	// Пробуем base64
	data, err := base64.StdEncoding.DecodeString(dataStr)
	if err == nil {
		return data, nil
	}
	// Пробуем hex
	data, err = hex.DecodeString(dataStr)
	if err == nil {
		return data, nil
	}
	return nil, fmt.Errorf("unknown encoding (not base64 or hex)")
}

func ParseCreateInstructionLog(dataStr string) (*CreateInstruction, error) {
	data, err := decodeDataString(dataStr)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("too short for discriminator")
	}
	discriminator := binary.LittleEndian.Uint64(data[:8])
	if discriminator != CREATE_DISCRIMINATOR {
		return nil, fmt.Errorf("not a CreateInstruction discriminator: %x", discriminator)
	}
	offset := 8
	readString := func() (string, error) {
		if offset+4 > len(data) {
			return "", fmt.Errorf("not enough data for string length")
		}
		l := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
		if offset+int(l) > len(data) {
			return "", fmt.Errorf("not enough data for string value")
		}
		val := string(data[offset : offset+int(l)])
		offset += int(l)
		return val, nil
	}
	readPubKey := func() (string, error) {
		if offset+32 > len(data) {
			return "", fmt.Errorf("not enough data for pubkey")
		}
		val := base58.Encode(data[offset : offset+32])
		offset += 32
		return val, nil
	}

	var ci CreateInstruction
	if ci.Name, err = readString(); err != nil {
		return nil, err
	}
	if ci.Symbol, err = readString(); err != nil {
		return nil, err
	}
	if ci.URI, err = readString(); err != nil {
		return nil, err
	}
	if ci.Mint, err = readPubKey(); err != nil {
		return nil, err
	}
	if ci.BondingCurve, err = readPubKey(); err != nil {
		return nil, err
	}
	if ci.User, err = readPubKey(); err != nil {
		return nil, err
	}
	if ci.Creator, err = readPubKey(); err != nil {
		return nil, err
	}
	return &ci, nil
}

func main() {
	// Пример: подставьте свою строку лога (hex или base64)
	// Пример base64: "bPh6MgK80P8LAAAACVB1bXAgRnVuAAMAAABQRlUAAAAidHRwczovL3BtLnhmeS5jb20vaW1nLnBuZ..."
	dataStr := "G3KpTd7rY3YRAAAAY2F0IG9uIHRvcCBvZiBwaWcJAAAAcM2oac2jZ82tQwAAAGh0dHBzOi8vaXBmcy5pby9pcGZzL1FtWjRiSnN3THVlcmF0SHFaaXpxVEhpSG44RVJFV2ZCYjRwVHBFMU5KN3RmcXMzil+iNccKjQw2Jl0o5BdLMhLgiTRk2OcG6JH+vPfIP0oCfGlSSXBX8TUF/2cHntnln9Zk6rWfqbzoyS1yjcdfzZio8H2p5LrHQNcvxiwT86sHWZTGDQm7HmdhLJaCeEnNmKjwfankusdA1y/GLBPzqwdZlMYNCbseZ2EsloJ4SSxHOWgAAAAAABDYR+PPAwAArCP8BgAAAAB4xftR0QIAAIDGpH6NAwA="

	ci, err := ParseCreateInstructionLog(dataStr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", ci)
}
