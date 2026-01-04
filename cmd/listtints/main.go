package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/EchoTools/evrFileTools/pkg/manifest"
	"github.com/EchoTools/evrFileTools/pkg/tint"
)

type typeStats struct {
	count      int
	totalSize  uint64
	assetTypes map[uint64]int
}

type assetStats struct {
	count int
	types map[uint64]int
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./cmd/listtints <manifest_file>")
		os.Exit(1)
	}

	path := os.Args[1]
	m, err := manifest.ReadFile(path)
	if err != nil {
		fmt.Printf("failed to read manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Manifest: %s\n", path)
	fmt.Printf("Packages: %d\n", m.PackageCount())
	fmt.Printf("Files:    %d\n", len(m.FrameContents))
	fmt.Printf("Frames:   %d\n\n", len(m.Frames))

	metaMap := make(map[[2]uint64]manifest.FileMetadata, len(m.Metadata))
	for _, md := range m.Metadata {
		metaMap[[2]uint64{md.TypeSymbol, md.FileSymbol}] = md
	}

	stats := make(map[uint64]*typeStats)
	assets := make(map[uint64]*assetStats)

	for idx, fc := range m.FrameContents {
		key := [2]uint64{fc.TypeSymbol, fc.FileSymbol}
		md := metaMap[key]

		ts, ok := stats[fc.TypeSymbol]
		if !ok {
			ts = &typeStats{assetTypes: make(map[uint64]int)}
			stats[fc.TypeSymbol] = ts
		}
		ts.count++
		ts.totalSize += uint64(fc.Size)
		ts.assetTypes[md.AssetType]++

		as, ok := assets[md.AssetType]
		if !ok {
			as = &assetStats{types: make(map[uint64]int)}
			assets[md.AssetType] = as
		}
		as.count++
		as.types[fc.TypeSymbol]++

		// Check if this FileSymbol matches a known tint hash
		if name := tint.LookupTintName(fc.FileSymbol); name != "" {
			fr := m.Frames[fc.FrameIndex]
			fmt.Printf("KNOWN TINT %-24s type=%016x file=%016x frame=%d pkg=%d off=%d comp=%d len=%d dataOff=%d size=%d assetType=%016x\n",
				name,
				fc.TypeSymbol,
				fc.FileSymbol,
				fc.FrameIndex,
				fr.PackageIndex,
				fr.Offset,
				fr.CompressedSize,
				fr.Length,
				fc.DataOffset,
				fc.Size,
				md.AssetType,
			)
			// Continue scanning; no break to allow multiple hits
			_ = idx // keep idx available for future debugging
		}
	}

	// Sort type symbols by count descending
	type kv struct {
		typeSymbol uint64
		count      int
		totalSize  uint64
	}

	list := make([]kv, 0, len(stats))
	for typeSym, st := range stats {
		list = append(list, kv{typeSymbol: typeSym, count: st.count, totalSize: st.totalSize})
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].count == list[j].count {
			return list[i].typeSymbol < list[j].typeSymbol
		}
		return list[i].count > list[j].count
	})

	max := 20
	if len(list) < max {
		max = len(list)
	}

	fmt.Println("Top type symbols by file count:")
	fmt.Println("typeSymbol              count    totalKB  assetTypes")
	for i := 0; i < max; i++ {
		ts := stats[list[i].typeSymbol]
		fmt.Printf("%016x  %7d  %8.1f  %v\n", list[i].typeSymbol, list[i].count, float64(list[i].totalSize)/1024.0, ts.assetTypes)
	}

	// AssetType histogram
	type akv struct {
		assetType uint64
		count     int
	}
	assetList := make([]akv, 0, len(assets))
	for a, st := range assets {
		assetList = append(assetList, akv{assetType: a, count: st.count})
	}
	sort.Slice(assetList, func(i, j int) bool {
		if assetList[i].count == assetList[j].count {
			return assetList[i].assetType < assetList[j].assetType
		}
		return assetList[i].count > assetList[j].count
	})

	maxA := 10
	if len(assetList) < maxA {
		maxA = len(assetList)
	}

	fmt.Println("\nTop asset types by file count:")
	fmt.Println("assetType              count    topTypeSymbols")
	for i := 0; i < maxA; i++ {
		st := assets[assetList[i].assetType]
		// collect top 3 typeSymbols for this assetType
		tsym := make([]kv, 0, len(st.types))
		for t, c := range st.types {
			tsym = append(tsym, kv{typeSymbol: t, count: c})
		}
		sort.Slice(tsym, func(i, j int) bool {
			if tsym[i].count == tsym[j].count {
				return tsym[i].typeSymbol < tsym[j].typeSymbol
			}
			return tsym[i].count > tsym[j].count
		})
		limit := 3
		if len(tsym) < limit {
			limit = len(tsym)
		}
		names := make([]string, 0, limit)
		for j := 0; j < limit; j++ {
			names = append(names, fmt.Sprintf("%016x(%d)", tsym[j].typeSymbol, tsym[j].count))
		}
		fmt.Printf("%016x  %7d  %v\n", assetList[i].assetType, assetList[i].count, names)
	}
}
