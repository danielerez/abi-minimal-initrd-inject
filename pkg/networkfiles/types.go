package networkfiles

// StaticNetworkConfigData is one file placed under /etc/assisted/network/ in the initrd cpio.
type StaticNetworkConfigData struct {
	FilePath     string
	FileContents string
}
