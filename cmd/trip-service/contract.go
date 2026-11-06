//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package main
package main

import "context"

type closer interface {
	Close(context.Context)
}
