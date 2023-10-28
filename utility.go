package main

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
)

/*
***************************************
		UTILITY FUNCTIONS FOR MAIN
***************************************
*/

const M = 64

func GenerateHash(input string) uint64{
	data := []byte(input)
	id := sha256.Sum256(data)
	unmoddedID := float64(binary.BigEndian.Uint64(id[:8]))
	modValue := float64(math.Pow(2, M))
	moddedID := math.Mod(unmoddedID, modValue)
	return uint64(moddedID)
}