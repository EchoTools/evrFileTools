// Command symhash computes and looks up EVR symbol hashes.
//
// Two algorithms are supported:
//
//   - symbol (default): CSymbol64Hash — case-insensitive CRC64 variant used for
//     asset IDs, replicated variable names, and general symbol hashing.
//
//   - sns: SNSMessageHash — two-stage pipeline for SNS protocol message type IDs.
//     Strips leading 'S' prefix before hashing, as confirmed from echovr.exe.
//
// Usage:
//
//	symhash arena                          # hash one string (symbol algo)
//	symhash -algo sns SNSLobbySmiteEntrant # hash using SNS algo
//	symhash -reverse 0xbeac1969cb7b8861    # look up hash in built-in wordlist
//	symhash -wordlist names.txt 0x...      # look up hash with custom wordlist
//	cat names.txt | symhash -              # hash stdin lines
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/EchoTools/evrFileTools/pkg/hash"
)

var (
	algo     string
	reverse  string
	wordlist string
)

func init() {
	flag.StringVar(&algo, "algo", "symbol", "Hash algorithm: symbol (CSymbol64) or sns (SNS message)")
	flag.StringVar(&reverse, "reverse", "", "Reverse-lookup a hash value (hex, e.g. 0xbeac1969cb7b8861)")
	flag.StringVar(&wordlist, "wordlist", "", "Path to wordlist file for reverse lookup (one name per line)")
}

func main() {
	flag.Parse()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func computeHash(s string) uint64 {
	switch algo {
	case "sns":
		return hash.SNSMessageHash(s)
	default:
		return hash.CSymbol64Hash(s)
	}
}

func run() error {
	// Reverse lookup mode
	if reverse != "" {
		target, err := parseHex(reverse)
		if err != nil {
			return fmt.Errorf("invalid hash %q: %w", reverse, err)
		}
		return runReverse(target)
	}

	args := flag.Args()

	// Read from stdin if '-' or no args
	if len(args) == 0 || (len(args) == 1 && args[0] == "-") {
		return hashLines(os.Stdin)
	}

	// Hash each argument
	for _, s := range args {
		h := computeHash(s)
		fmt.Printf("0x%016x  %s\n", h, s)
	}
	return nil
}

func hashLines(r *os.File) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		h := computeHash(line)
		fmt.Printf("0x%016x  %s\n", h, line)
	}
	return scanner.Err()
}

func runReverse(target uint64) error {
	// Build lookup table from wordlist + built-in names
	table := make(map[uint64]string)

	// Load built-in wordlist
	for _, name := range builtinNames {
		table[computeHash(name)] = name
	}

	// Load user wordlist if provided
	if wordlist != "" {
		f, err := os.Open(wordlist)
		if err != nil {
			return fmt.Errorf("open wordlist: %w", err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			table[computeHash(line)] = line
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read wordlist: %w", err)
		}
	}

	if name, ok := table[target]; ok {
		fmt.Printf("0x%016x  %s\n", target, name)
		return nil
	}

	fmt.Printf("0x%016x  <unknown>\n", target)
	return nil
}

func parseHex(s string) (uint64, error) {
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	return strconv.ParseUint(s, 16, 64)
}

// builtinNames is a seed wordlist of known EVR symbol names.
// Extend via -wordlist for broader coverage.
var builtinNames = []string{
	// Asset types (from pkg/naming)
	"texture_dds",
	"texture_bc_raw",
	"texture_meta",
	"audio_ref",
	"asset_ref",

	// Level/environment assets
	"social_2.0_arena",
	"social_2.0_private",
	"social_2.0_npe",
	"arena_environment",
	"lobby_environment",
	"courtyard_environment",
	"environment_lighting",
	"environment_props",
	"environment_decals",
	"environment_particles",
	"environment_effects",

	// SNS message types (full names — SNSMessageHash strips the 'S')
	"SNSLobbySmiteEntrant",
	"SBroadcasterIntroduceFinEvent",
	"SNSRefreshProfileForUser",
	"SNSLobbyCreateSessionRequestv9",
	"SNSLobbySessionSuccessv5",
	"SNSLobbyFindSessionsRequest",
	"SNSLobbyMatchmakerStatusRequest",
	"SNSLobbyPingRequest",
	"SNSLobbyPingResponse",

	// Replicated variable names (common game state)
	"player",
	"disc",
	"arena",
	"team_blue",
	"team_orange",
	"goal",
	"position",
	"velocity",
	"rotation",
	"score",

	// Cosmetic/loadout slots
	"unified",
	"orange_team",
	"blue_team",
	"combat",
	"social",
}
