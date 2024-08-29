package basic_workflows

import (
	"errors"
	"io/fs"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/snyk/go-application-framework/pkg/configuration"
	"github.com/snyk/go-application-framework/pkg/workflow"
	"github.com/spf13/pflag"

	"github.com/snyk/cli/cliv2/internal/cliv2"
	"github.com/snyk/cli/cliv2/internal/proxy"
	"github.com/snyk/cli/cliv2/internal/utils"
)

var caSingleton *proxy.CaData
var caMutex sync.Mutex

var WORKFLOWID_GLOBAL_CLEANUP workflow.Identifier = workflow.NewWorkflowIdentifier("internal.cleanup")

func initCleanup(engine workflow.Engine) error {
	entry, err := engine.Register(WORKFLOWID_GLOBAL_CLEANUP, workflow.ConfigurationOptionsFromFlagset(pflag.NewFlagSet("cleanup", pflag.ContinueOnError)), globalCleanupWorkflow)
	if err != nil {
		return err
	}
	entry.SetVisibility(false)

	return nil
}

func globalCleanupWorkflow(
	invocation workflow.InvocationContext,
	_ []workflow.Data,
) (output []workflow.Data, err error) {
	logger := invocation.GetEnhancedLogger()
	config := invocation.GetConfiguration()

	CleanupGlobalCertAuthority(logger)
	CleanupGlobalTempDirectory(config, logger)

	return output, err
}

func CleanupGlobalCertAuthority(debugLogger *zerolog.Logger) {
	caMutex.Lock()
	defer caMutex.Unlock()
	if caSingleton != nil {
		err := os.Remove(caSingleton.CertFile)
		if err != nil {
			debugLogger.Print("Failed to delete temporary certificate file: ", caSingleton.CertFile)
			debugLogger.Print(err)
		} else {
			debugLogger.Print("Deleted temporary certificate file: ", caSingleton.CertFile)
		}

		caSingleton = nil
	}
}

func GetGlobalCertAuthority(config configuration.Configuration, debugLogger *zerolog.Logger) (proxy.CaData, error) {
	caMutex.Lock()
	defer caMutex.Unlock()

	createCA := false

	if caSingleton == nil {
		createCA = true
	} else if _, existsError := os.Stat(caSingleton.CertFile); errors.Is(existsError, fs.ErrNotExist) { // certificate file does not exist
		if len(caSingleton.CertPem) > 0 && len(caSingleton.CertFile) > 0 { // try to re-create file
			debugLogger.Printf("Restoring temporary certificate file: %s", caSingleton.CertFile)
			utils.WriteToFile(caSingleton.CertFile, caSingleton.CertPem)
		} else { // create a new CA
			debugLogger.Warn().Msg("Used Certificate Authority is not existing anymore")
			createCA = true
		}
	}

	if createCA {
		debugLogger.Print("Creating new Certificate Authority")
		tmp, err := proxy.InitCA(config, cliv2.GetFullVersion(), debugLogger)
		if err != nil {
			return proxy.CaData{}, err
		}
		caSingleton = tmp
	}

	return *caSingleton, nil
}

func CleanupGlobalTempDirectory(config configuration.Configuration, debugLogger *zerolog.Logger) {
	tmpDirectory := utils.GetTemporaryDirectory(config.GetString(configuration.CACHE_PATH), cliv2.GetFullVersion())
	err := os.RemoveAll(tmpDirectory)
	if err != nil {
		debugLogger.Print("Failed to delete temporary directory: ", tmpDirectory)
		debugLogger.Print(err)
		return
	}

	debugLogger.Print("Deleted temporary directory: ", tmpDirectory)
}
