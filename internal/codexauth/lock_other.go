//go:build !unix

package codexauth

func lockAuth(authPath string) (func(), error) {
	_ = authPath
	return func() {}, nil
}
