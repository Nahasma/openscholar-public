// Package config provides configuration loading, defaults, and persistence for OpenScholar.
// The implementation is split across:
//   - types.go   — all struct/enum type definitions
//   - runtime.go — singleton state (Get/Reset/WorkingDirectory/LogDir)
//   - load.go    — Load(), mergeConfigFile(), applyAgentDefaults()
//   - env.go     — environment variable overlay and exportAPIKeysToEnv
//   - defaults.go — DefaultHarnessConfig, DefaultExperimentConfig, defaultModel*
//   - save.go    — ConfigFilePath, InitConfigFile, SaveAgentModel, SaveFull
package config
