//go:build windows

package setup

func setupRunningElevated() bool {
	// Windows does not expose a portable standard-library equivalent to
	// os.Geteuid. The installer still uses a private user prefix and never
	// requests elevation; administrator-token detection remains a platform QA
	// item rather than an implicit privilege escalation.
	return false
}
