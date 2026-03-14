package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/EchoTools/evrFileTools/pkg/manifest"
	"github.com/EchoTools/evrFileTools/pkg/naming"
)

func runDiff() error {
	if diffManifestA == "" || diffManifestB == "" {
		return fmt.Errorf("diff mode requires -manifest-a and -manifest-b")
	}

	ma, err := manifest.ReadFile(diffManifestA)
	if err != nil {
		return fmt.Errorf("read manifest A: %w", err)
	}
	mb, err := manifest.ReadFile(diffManifestB)
	if err != nil {
		return fmt.Errorf("read manifest B: %w", err)
	}

	// Build maps: (typeSymbol, fileSymbol) → size
	type fileKey struct {
		typeSymbol int64
		fileSymbol int64
	}

	aFiles := make(map[fileKey]uint32, len(ma.FrameContents))
	for _, fc := range ma.FrameContents {
		aFiles[fileKey{fc.TypeSymbol, fc.FileSymbol}] = fc.Size
	}

	bFiles := make(map[fileKey]uint32, len(mb.FrameContents))
	for _, fc := range mb.FrameContents {
		bFiles[fileKey{fc.TypeSymbol, fc.FileSymbol}] = fc.Size
	}

	type diffEntry struct {
		key    fileKey
		status string // "added", "removed", "modified"
		sizeA  uint32
		sizeB  uint32
	}
	var diffs []diffEntry

	// Find removed and modified
	for k, sizeA := range aFiles {
		if sizeB, ok := bFiles[k]; ok {
			if sizeA != sizeB {
				diffs = append(diffs, diffEntry{k, "modified", sizeA, sizeB})
			}
		} else {
			diffs = append(diffs, diffEntry{k, "removed", sizeA, 0})
		}
	}

	// Find added
	for k, sizeB := range bFiles {
		if _, ok := aFiles[k]; !ok {
			diffs = append(diffs, diffEntry{k, "added", 0, sizeB})
		}
	}

	if len(diffs) == 0 {
		fmt.Println("No differences found.")
		return nil
	}

	// Sort: status (added/removed/modified), then type, then file
	statusOrder := map[string]int{"added": 0, "removed": 1, "modified": 2}
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].status != diffs[j].status {
			return statusOrder[diffs[i].status] < statusOrder[diffs[j].status]
		}
		if diffs[i].key.typeSymbol != diffs[j].key.typeSymbol {
			return diffs[i].key.typeSymbol < diffs[j].key.typeSymbol
		}
		return diffs[i].key.fileSymbol < diffs[j].key.fileSymbol
	})

	// Summary by type and status
	type summary struct {
		added, removed, modified int
		addedBytes, removedBytes int64
	}
	typeSummary := make(map[int64]*summary)
	totals := &summary{}

	for _, d := range diffs {
		s, ok := typeSummary[d.key.typeSymbol]
		if !ok {
			s = &summary{}
			typeSummary[d.key.typeSymbol] = s
		}
		switch d.status {
		case "added":
			s.added++
			totals.added++
			s.addedBytes += int64(d.sizeB)
			totals.addedBytes += int64(d.sizeB)
		case "removed":
			s.removed++
			totals.removed++
			s.removedBytes += int64(d.sizeA)
			totals.removedBytes += int64(d.sizeA)
		case "modified":
			s.modified++
			totals.modified++
		}
	}

	// Print per-type summary
	fmt.Printf("Manifest A: %s (%d files)\n", diffManifestA, ma.FileCount())
	fmt.Printf("Manifest B: %s (%d files)\n\n", diffManifestB, mb.FileCount())

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\t+ADDED\t-REMOVED\t~MODIFIED")
	fmt.Fprintln(w, "----\t------\t--------\t---------")

	// Sort types for deterministic output
	var typeSymbols []int64
	for ts := range typeSummary {
		typeSymbols = append(typeSymbols, ts)
	}
	sort.Slice(typeSymbols, func(i, j int) bool { return typeSymbols[i] < typeSymbols[j] })

	for _, ts := range typeSymbols {
		s := typeSummary[ts]
		typeName := naming.TypeSymbol(ts).String()
		added := ""
		removed := ""
		modified := ""
		if s.added > 0 {
			added = fmt.Sprintf("+%d (%s)", s.added, formatBytes(s.addedBytes))
		}
		if s.removed > 0 {
			removed = fmt.Sprintf("-%d (%s)", s.removed, formatBytes(s.removedBytes))
		}
		if s.modified > 0 {
			modified = fmt.Sprintf("~%d", s.modified)
		}
		if added == "" && removed == "" && modified == "" {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", typeName, added, removed, modified)
	}

	fmt.Fprintln(w, "────\t──────\t────────\t─────────")
	fmt.Fprintf(w, "TOTAL\t+%d (%s)\t-%d (%s)\t~%d\n",
		totals.added, formatBytes(totals.addedBytes),
		totals.removed, formatBytes(totals.removedBytes),
		totals.modified)
	w.Flush()

	// Optionally print verbose file list
	if verbose {
		fmt.Printf("\n=== File Changes ===\n")
		w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, d := range diffs {
			typeName := naming.TypeSymbol(d.key.typeSymbol).String()
			switch d.status {
			case "added":
				fmt.Fprintf(w2, "+ %s\t%016x\t%s\n", typeName, uint64(d.key.fileSymbol), formatBytes(int64(d.sizeB)))
			case "removed":
				fmt.Fprintf(w2, "- %s\t%016x\t%s\n", typeName, uint64(d.key.fileSymbol), formatBytes(int64(d.sizeA)))
			case "modified":
				fmt.Fprintf(w2, "~ %s\t%016x\t%s → %s\n", typeName, uint64(d.key.fileSymbol),
					formatBytes(int64(d.sizeA)), formatBytes(int64(d.sizeB)))
			}
		}
		w2.Flush()
	}

	return nil
}
