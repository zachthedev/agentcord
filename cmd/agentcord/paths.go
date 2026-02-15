package main

import "tools.zach/dev/agentcord/internal/paths"

// ///////////////////////////////////////////////
// Path Aliases
// ///////////////////////////////////////////////

// DataPaths aliases [paths.DataDir] into the main package so that daemon code
// can reference path helpers without qualifying the internal package name.
// This file has no build constraints because path construction is
// platform-independent; [filepath.Join] in [paths.DataDir] handles OS-specific
// separators automatically.
type DataPaths = paths.DataDir
