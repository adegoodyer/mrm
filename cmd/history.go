package cmd

import (
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/adegoodyer/mrm/internal/logger"
	"github.com/adegoodyer/mrm/internal/paths"
	"github.com/adegoodyer/mrm/internal/store"
)

// record appends something mrm changed to the history, which the web UI's
// Activity page shows. A failure to write it is a warning, not an error:
// the change itself is done.
func record(run store.Run) {
	if err := store.AppendRun(paths.History(), run); err != nil {
		fmt.Fprintln(os.Stderr, "warning: not written to the history: "+err.Error())
		logger.L.Warn("history not written", zap.String("op", run.Op), zap.Error(err))
	}
}
