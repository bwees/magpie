// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build race

package parse

// The race detector slows regexp2 down about a hundredfold, so the gate soundness
// test samples its corpus under -race.
const raceEnabled = true
