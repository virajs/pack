package pack

type CreateBuilderFlags struct {
	RepoName        string
	BuilderTomlPath string
}

func (c *CreateBuilderFlags) Run() error {
	return nil
}
