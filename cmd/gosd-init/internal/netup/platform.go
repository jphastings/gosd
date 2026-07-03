package netup

// Platform bundles the real implementations of Links and DHCPClient.
// NewPlatform is implemented once per build tag (platform_linux.go,
// platform_other.go) so main.go can wire it up without caring which OS
// it's running on.
type Platform struct {
	Links Links
	DHCP  DHCPClient
}
