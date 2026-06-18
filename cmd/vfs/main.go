package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"vfs-layer/internal/check"
	"vfs-layer/internal/config"
	"vfs-layer/internal/ingest"
	"vfs-layer/internal/storage"
	"vfs-layer/internal/view"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "mount":
		err = runMount(os.Args[2:])
	case "check":
		err = runCheck(os.Args[2:])
	case "repair":
		err = runRepair(os.Args[2:])
	case "ingest":
		err = runIngest(os.Args[2:])
	case "encode-name":
		err = runEncodeName(os.Args[2:])
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vfs:", err)
		os.Exit(exitCode(err))
	}
}

func runMount(args []string) error {
	fs := flag.NewFlagSet("mount", flag.ExitOnError)
	common := commonFlags(fs)
	debug := fs.Bool("debug", false, "enable FUSE debug logging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadCommon(common)
	if err != nil {
		return err
	}
	store, err := storage.New(cfg.StorageRoot, cfg)
	if err != nil {
		return err
	}
	server, err := view.Mount(cfg.ViewRoot, store, *debug)
	if err != nil {
		return err
	}
	fmt.Printf("mounted %s over %s\n", cfg.ViewRoot, cfg.StorageRoot)
	server.Wait()
	return nil
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	common := commonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadCommon(common)
	if err != nil {
		return err
	}
	store, err := storage.New(cfg.StorageRoot, cfg)
	if err != nil {
		return err
	}
	issues, err := check.Scan(store)
	if err != nil {
		return err
	}
	for _, issue := range issues {
		fmt.Printf("%s\t%s\t%s\n", issue.Severity, issue.Path, issue.Message)
	}
	if check.HasErrors(issues) {
		return errCheckFailed
	}
	return nil
}

func runRepair(args []string) error {
	fs := flag.NewFlagSet("repair", flag.ExitOnError)
	common := commonFlags(fs)
	apply := fs.Bool("apply", false, "remove orphan sidecar metadata")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadCommon(common)
	if err != nil {
		return err
	}
	store, err := storage.New(cfg.StorageRoot, cfg)
	if err != nil {
		return err
	}
	if !*apply {
		issues, err := check.Scan(store)
		if err != nil {
			return err
		}
		for _, issue := range issues {
			fmt.Printf("%s\t%s\t%s\n", issue.Severity, issue.Path, issue.Message)
		}
		fmt.Println("dry run only; pass --apply to remove orphan sidecars")
		return nil
	}
	removed, err := check.RemoveOrphanSidecars(store)
	if err != nil {
		return err
	}
	for _, path := range removed {
		fmt.Printf("removed\t%s\n", path)
	}
	return nil
}

func runIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	common := commonFlags(fs)
	manifest := fs.String("manifest", "", "JSONL ingest manifest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifest == "" {
		return errors.New("--manifest is required")
	}
	cfg, err := loadCommon(common)
	if err != nil {
		return err
	}
	store, err := storage.New(cfg.StorageRoot, cfg)
	if err != nil {
		return err
	}
	return ingest.Run(store, *manifest)
}

func runEncodeName(args []string) error {
	fs := flag.NewFlagSet("encode-name", flag.ExitOnError)
	common := commonFlags(fs)
	isDir := fs.Bool("dir", false, "encode as a directory name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("encode-name requires exactly one name")
	}
	cfg, err := loadCommon(common)
	if err != nil {
		return err
	}
	store, err := storage.New(cfg.StorageRoot, cfg)
	if err != nil {
		return err
	}
	fmt.Println(store.EncodeName(fs.Arg(0), *isDir))
	return nil
}

type commonFlagValues struct {
	config  *string
	storage *string
	view    *string
}

func commonFlags(fs *flag.FlagSet) commonFlagValues {
	return commonFlagValues{
		config:  fs.String("config", "", "plain YAML config path"),
		storage: fs.String("storage", "", "storage root"),
		view:    fs.String("view", "", "FUSE mountpoint"),
	}
}

func loadCommon(flags commonFlagValues) (config.Config, error) {
	cfg, err := config.Load(*flags.config)
	if err != nil {
		return cfg, err
	}
	if *flags.storage != "" {
		cfg.StorageRoot = *flags.storage
	}
	if *flags.view != "" {
		cfg.ViewRoot = *flags.view
	}
	cfg.ApplyDefaults()
	return cfg, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  vfs mount [--config path] [--storage path] [--view path]
  vfs check [--config path] [--storage path]
  vfs repair [--config path] [--storage path] [--apply]
  vfs ingest --manifest manifest.jsonl [--config path] [--storage path]
  vfs encode-name [--config path] [--storage path] [--dir] <name>
  vfs version`)
}

var errCheckFailed = errors.New("check failed")

func exitCode(err error) int {
	if errors.Is(err, errCheckFailed) {
		return 1
	}
	return 2
}
