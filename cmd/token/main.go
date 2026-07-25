// Command token prints a signed dev bearer token for api.rest and manual calls.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/alkmc/storefront/internal/auth/authtest"
	"github.com/alkmc/storefront/internal/config"
	"github.com/google/uuid"
)

func main() {
	sub := flag.String("sub", "", "user id claim, a fresh UUID when empty")
	ttl := flag.Duration("ttl", time.Hour, "token lifetime")
	flag.Parse()

	cfg, err := config.LoadAuth()
	if err != nil {
		fail("load auth: " + err.Error())
	}

	id, err := subID(*sub)
	if err != nil {
		fail("sub: " + err.Error())
	}

	_, _ = fmt.Fprintln(os.Stdout, authtest.Token(cfg.JWTSecret.Reveal(), id, time.Now().Add(*ttl)))
}

func subID(sub string) (uuid.UUID, error) {
	if sub == "" {
		return uuid.NewV7()
	}
	return uuid.Parse(sub)
}

func fail(msg string) {
	_, _ = fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
