package log

import (
	"os"

	"github.com/rs/zerolog"
)

var Logger zerolog.Logger

func SetLevelDebug() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
}
func SetLevelInfo() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
}
