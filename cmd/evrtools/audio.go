package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/EchoTools/evrFileTools/pkg/audio"
)

const audioHeaderReadSize = 64

type audioFileResult struct {
	path   string
	info   *audio.AudioInfo
	format audio.AudioFormat
}

type audioFormatSummary struct {
	format     audio.AudioFormat
	count      int
	sampleRate uint32 // most common or first seen
	channels   uint16 // most common or first seen
	paths      []string
}

func runAudio() error {
	if inputDir == "" {
		return fmt.Errorf("audio mode requires -input")
	}

	var results []audioFileResult

	err := filepath.WalkDir(inputDir, func(filePath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		// Skip sidecar files
		if strings.HasSuffix(filePath, ".meta") {
			return nil
		}

		header, err := readAudioHeader(filePath)
		if err != nil {
			return nil // skip unreadable files
		}

		format := audio.DetectFormat(header)
		if format == audio.FormatUnknown {
			return nil
		}

		info := audio.ParseInfo(header)

		results = append(results, audioFileResult{
			path:   filePath,
			info:   info,
			format: format,
		})

		return nil
	})
	if err != nil {
		return fmt.Errorf("walk directory: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No audio files found.")
		return nil
	}

	if verbose {
		printVerboseAudio(results)
	}

	printAudioSummary(results)
	return nil
}

func readAudioHeader(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, audioHeaderReadSize)
	n, _ := f.Read(buf)
	return buf[:n], nil
}

func printVerboseAudio(results []audioFileResult) {
	fmt.Println("=== Audio Files ===")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FORMAT\tSAMPLE RATE\tCHANNELS\tBIT DEPTH\tDURATION\tPATH")
	fmt.Fprintln(w, "------\t-----------\t--------\t---------\t--------\t----")
	for _, r := range results {
		sampleRate := "-"
		channels := "-"
		bitDepth := "-"
		duration := "-"
		if r.info != nil {
			if r.info.SampleRate > 0 {
				sampleRate = fmt.Sprintf("%d Hz", r.info.SampleRate)
			}
			if r.info.Channels > 0 {
				channels = fmt.Sprintf("%d ch", r.info.Channels)
			}
			if r.info.BitDepth > 0 {
				bitDepth = fmt.Sprintf("%d bit", r.info.BitDepth)
			}
			if r.info.Duration > 0 {
				duration = fmt.Sprintf("%.2fs", r.info.Duration)
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.format, sampleRate, channels, bitDepth, duration, r.path)
	}
	w.Flush()
	fmt.Println()
}

func printAudioSummary(results []audioFileResult) {
	// Group by format
	summaries := make(map[audio.AudioFormat]*audioFormatSummary)

	for _, r := range results {
		s, ok := summaries[r.format]
		if !ok {
			s = &audioFormatSummary{format: r.format}
			summaries[r.format] = s
		}
		s.count++
		s.paths = append(s.paths, r.path)
		if r.info != nil && s.sampleRate == 0 && r.info.SampleRate > 0 {
			s.sampleRate = r.info.SampleRate
		}
		if r.info != nil && s.channels == 0 && r.info.Channels > 0 {
			s.channels = r.info.Channels
		}
	}

	// Sort by count descending
	var sorted []*audioFormatSummary
	for _, s := range summaries {
		sorted = append(sorted, s)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	fmt.Printf("=== Audio Summary (%d files total) ===\n", len(results))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FORMAT\tFILES\tSAMPLE RATE\tCHANNELS\tPATH (first 5 shown)")
	fmt.Fprintln(w, "------\t-----\t-----------\t--------\t--------------------")
	for _, s := range sorted {
		sampleRate := "-"
		channels := "-"
		if s.sampleRate > 0 {
			sampleRate = fmt.Sprintf("%d Hz", s.sampleRate)
		}
		if s.channels > 0 {
			channels = fmt.Sprintf("%d ch", s.channels)
		}

		// Show up to 5 example paths (relative if possible)
		shown := s.paths
		if len(shown) > 5 {
			shown = shown[:5]
		}
		pathStr := strings.Join(shown, ", ")

		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
			s.format, s.count, sampleRate, channels, pathStr)
	}
	w.Flush()
}
