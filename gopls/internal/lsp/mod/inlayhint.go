// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package mod

import (
	"context"
	"fmt"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/gopls/internal/lsp/protocol"
	"golang.org/x/tools/gopls/internal/lsp/source"
)

func InlayHint(ctx context.Context, snapshot source.Snapshot, fh source.FileHandle, rng protocol.Range) ([]protocol.InlayHint, error) {
	// Inlay hints are enabled if the client supports them.
	pm, err := snapshot.ParseMod(ctx, fh)
	if err != nil {
		return nil, err
	}
	return unexpectedVersion(ctx, snapshot, pm), nil
}

// Compare the version of the module used in the snapshot's metadata with the
// version requested by the module, in both cases, taking replaces into account.
// Produce an InlayHint when the version is the module is not the one usedd.
func unexpectedVersion(ctx context.Context, snapshot source.Snapshot, pm *source.ParsedModule) []protocol.InlayHint {
	var ans []protocol.InlayHint
	if pm.File == nil {
		return nil
	}
	replaces := make(map[string]*modfile.Replace)
	requires := make(map[string]*modfile.Require)
	for _, x := range pm.File.Replace {
		replaces[x.Old.Path] = x
	}
	for _, x := range pm.File.Require {
		requires[x.Mod.Path] = x
	}
	am, _ := snapshot.AllMetadata(ctx)
	seen := make(map[string]bool)
	for _, meta := range am {
		if meta == nil || meta.Module == nil || seen[meta.Module.Path] {
			continue
		}
		seen[meta.Module.Path] = true
		metaMod := meta.Module
		metaVersion := metaMod.Version
		if metaMod.Replace != nil {
			metaVersion = metaMod.Replace.Version
		}
		// These versions can be blank, as in gopls/go.mod's local replace
		if oldrepl, ok := replaces[metaMod.Path]; ok && oldrepl.New.Version != metaVersion {
			ih := genHint(oldrepl.Syntax, oldrepl.New.Version, metaVersion, pm.Mapper)
			if ih != nil {
				ans = append(ans, *ih)
			}
		} else if oldreq, ok := requires[metaMod.Path]; ok && oldreq.Mod.Version != metaVersion {
			// maybe it was replaced:
			if _, ok := replaces[metaMod.Path]; ok {
				continue
			}
			ih := genHint(oldreq.Syntax, oldreq.Mod.Version, metaVersion, pm.Mapper)
			if ih != nil {
				ans = append(ans, *ih)
			}
		}
	}
	return ans
}

func genHint(mline *modfile.Line, oldVersion, newVersion string, m *protocol.Mapper) *protocol.InlayHint {
	x := mline.End.Byte // the parser has removed trailing whitespace and comments (see modfile_test.go)
	x -= len(mline.Token[len(mline.Token)-1])
	line, err := m.OffsetPosition(x)
	if err != nil {
		return nil
	}
	part := protocol.InlayHintLabelPart{
		Value: newVersion,
		Tooltip: &protocol.OrPTooltipPLabel{
			Value: fmt.Sprintf("used metadata's version %s rather than go.mod's version %s", newVersion, oldVersion),
		},
	}
	rng, err := m.OffsetRange(x, mline.End.Byte)
	if err != nil {
		return nil
	}
	te := protocol.TextEdit{
		Range:   rng,
		NewText: newVersion,
	}
	return &protocol.InlayHint{
		Position:     line,
		Label:        []protocol.InlayHintLabelPart{part},
		Kind:         protocol.Parameter,
		PaddingRight: true,
		TextEdits:    []protocol.TextEdit{te},
	}
}
