package service

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
	"testing"
)

func TestPasswordEncrypt(t *testing.T) {
	password := []byte("@12345678a")
	encrypted, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	assert.NoError(t, err)
	fmt.Printf("🚀 ~ file: user_test.go ~ line 13 ~ encrypted: %#v\n", string(encrypted))
	err = bcrypt.CompareHashAndPassword(encrypted, password)
	assert.NoError(t, err)

}
