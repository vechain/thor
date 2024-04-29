package datagen

import (
	mathrand "math/rand"
)

func RandInt() int {
	return mathrand.Int()
}

func RandIntN(n int) int {
	return mathrand.Intn(n)
}
