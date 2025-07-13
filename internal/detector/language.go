package detector

type Language interface {
	Name() string
	Detect(projectPath string) bool
	Install(projectPath string, config Config) error
	GetLogPaths(projectPath string) []string
}

type Config struct {
	LogLevel   string `json:"log_level"`
	OutputDir  string `json:"output_dir"`
	MaxFiles   int    `json:"max_files"`
	DateFormat string `json:"date_format"`
}

var SupportedLanguages = []Language{
	&GoLanguage{},
	// Future languages will go here
}

func DetectLanguage(projectPath string) Language {
	for _, lang := range SupportedLanguages {
		if lang.Detect(projectPath) {
			return lang
		}
	}
	return nil
}
