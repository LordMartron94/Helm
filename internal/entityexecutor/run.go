package entityexecutor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"helm/internal/cache"
	"helm/internal/ir"
)

// EntityRunRequest is a spawn request for one entity build step.
type EntityRunRequest struct {
	EntityName string
	StepIndex  int
	Argv       []string
	WorkDir    string
	Env        map[string]string
}

// EntityRunHandler executes one entity spawn request.
type EntityRunHandler func(req EntityRunRequest) error

// EntityExecutorOptions configures native entity execution.
type EntityExecutorOptions struct {
	RunHandler      EntityRunHandler
	CacheRoot       string
	DisableCache    bool
	ownedCacheStore *cache.EntityCacheStore
}

// EntityExecutorDefaultRunHandler spawns argv without a shell.
func EntityExecutorDefaultRunHandler(req EntityRunRequest) error {
	if len(req.Argv) == 0 {
		return fmt.Errorf("entity '%s' step %d: empty argv", req.EntityName, req.StepIndex)
	}

	cmd := exec.Command(req.Argv[0], req.Argv[1:]...)
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), entityEnvPairs(req.Env)...)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("entity '%s' step %d: %w: %s", req.EntityName, req.StepIndex, err, stderr.String())
	}
	return nil
}

func entityEnvPairs(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}

// EntityExecutorRunGraph builds and runs all entities required by roots.
func EntityExecutorRunGraph(
	builtIR ir.HelmIR,
	roots []ir.HelmLabel,
	opts EntityExecutorOptions,
) error {
	plan, err := EntityBuildExecutionPlan(builtIR, roots)
	if err != nil {
		return err
	}

	handler := opts.RunHandler
	if handler == nil {
		handler = EntityExecutorDefaultRunHandler
	}

	runOpts := opts
	if !runOpts.DisableCache && runOpts.ownedCacheStore == nil {
		opened, openErr := cache.EntityCacheStoreOpen(entityCacheRoot(builtIR, runOpts.CacheRoot))
		if openErr != nil {
			return openErr
		}
		runOpts.ownedCacheStore = opened
		defer cache.EntityCacheStoreClose(runOpts.ownedCacheStore)
	}

	for _, phase := range plan.Phases {
		for _, entityKey := range phase {
			if err := entityExecutorRunOne(builtIR, entityKey, handler, runOpts); err != nil {
				return err
			}
		}
	}
	return nil
}

func entityExecutorRunOne(
	builtIR ir.HelmIR,
	entityKey string,
	handler EntityRunHandler,
	opts EntityExecutorOptions,
) error {
	plan, err := EntityExpandAdapter(builtIR.SourceDirectory, builtIR, entityKey)
	if err != nil {
		return err
	}

	primaryOutput := entityAdapterPrimaryOutputPath(plan)
	if !opts.DisableCache && primaryOutput != "" {
		stateFingerprint, fpErr := EntityCacheFingerprint(builtIR, entityKey, primaryOutput, plan.SourcePaths)
		if fpErr == nil {
			if hit, gateErr := entityCacheGate(opts.ownedCacheStore, entityKey, stateFingerprint, primaryOutput); gateErr == nil && hit {
				return nil
			}
		}
	}

	workDir := builtIR.SourceDirectory
	for stepIndex, step := range plan.Steps {
		if err := entityAdapterMkdirPaths(step.OutputPaths); err != nil {
			return err
		}
		if err := handler(EntityRunRequest{
			EntityName: entityKey,
			StepIndex:  stepIndex,
			Argv:       step.Argv,
			WorkDir:    workDir,
			Env:        step.Env,
		}); err != nil {
			return err
		}
	}

	if !opts.DisableCache && primaryOutput != "" {
		stateFingerprint, fpErr := EntityCacheFingerprint(builtIR, entityKey, primaryOutput, plan.SourcePaths)
		if fpErr == nil {
			return entityCacheRecord(opts.ownedCacheStore, entityKey, stateFingerprint, primaryOutput)
		}
	}
	return nil
}

func stringsJoinFlags(flags []string) string {
	if len(flags) == 0 {
		return ""
	}
	out := flags[0]
	for i := 1; i < len(flags); i++ {
		out += " " + flags[i]
	}
	return out
}

// EntityExecutorEnsureForTarget builds entity dependencies required by an entry target.
func EntityExecutorEnsureForTarget(
	builtIR ir.HelmIR,
	entryTarget string,
	opts EntityExecutorOptions,
) error {
	if builtIR.Mode != ir.HelmModeWorkspace {
		return nil
	}
	roots := EntityRootsFromTargetDeps(builtIR, entryTarget)
	if len(roots) == 0 {
		return nil
	}
	return EntityExecutorRunGraph(builtIR, roots, opts)
}

// EntityExecutorLegacyMode reports whether IR should use the v1 target executor only.
func EntityExecutorLegacyMode(builtIR ir.HelmIR) bool {
	return builtIR.Mode == ir.HelmModeLegacy
}

// EntityExecutorMkdirOutput ensures output directories exist before spawn.
func EntityExecutorMkdirOutput(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
