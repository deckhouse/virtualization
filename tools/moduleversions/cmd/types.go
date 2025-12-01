package cmd

// Config holds the configuration for the command.
type Config struct {
	Channel       string
	Version       string
	ModuleName    string
	Attempt       int
	CheckReleases bool
	CheckDocs     bool
}
