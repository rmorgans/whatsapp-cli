package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vicentereig/whatsapp-cli/internal/commands"
	"github.com/vicentereig/whatsapp-cli/internal/output"
	"golang.org/x/term"
)

const defaultTimeout = 5 * time.Minute

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// Registry holds the root cobra command and tracks registered commands.
type Registry struct {
	version    string
	storeDir   string
	formatFlag string // raw --format flag value
	app        *commands.App
	exitCode   int
	root       *cobra.Command
	parents    map[string]*cobra.Command
	ids        map[string]bool
	paths      map[string]bool
	writer     io.Writer // output destination; defaults to os.Stdout
}

// NewRegistry creates a new Registry with an initialised root command.
func NewRegistry() *Registry {
	r := &Registry{
		parents: make(map[string]*cobra.Command),
		ids:     make(map[string]bool),
		paths:   make(map[string]bool),
		writer:  os.Stdout,
	}
	r.root = newRootCmd(r)
	return r
}

// SetVersion sets the CLI version string.
func (r *Registry) SetVersion(v string) { r.version = v }

// Version returns the CLI version string.
func (r *Registry) Version() string { return r.version }

// Root returns the root cobra command for testing.
func (r *Registry) Root() *cobra.Command { return r.root }

// SetWriter overrides the output writer (default os.Stdout). Intended for tests.
func (r *Registry) SetWriter(w io.Writer) { r.writer = w }

// ---------------------------------------------------------------------------
// newRootCmd
// ---------------------------------------------------------------------------

func newRootCmd(r *Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "whatsapp-cli",
		Short:         "Command line interface for WhatsApp",
		Long:          "WhatsApp CLI - send messages, sync history, search contacts and chats.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetOut(os.Stderr)
	cmd.SetErr(os.Stderr)
	cmd.PersistentFlags().StringVar(&r.storeDir, "store", "./store", "storage directory")
	cmd.PersistentFlags().StringVar(&r.formatFlag, "format", "json", "output format: json, human, or auto")
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		switch r.formatFlag {
		case "json", "human", "auto":
			return nil
		default:
			return fmt.Errorf("--format must be one of [json, human, auto], got %q", r.formatFlag)
		}
	}
	return cmd
}

// ---------------------------------------------------------------------------
// RegisterParent
// ---------------------------------------------------------------------------

// RegisterParent registers a non-leaf parent command.
func (r *Registry) RegisterParent(spec ParentSpec) {
	if spec.Path.Name() == "" {
		panic("registry.RegisterParent: path must not be zero-value (empty name)")
	}
	parent := r.ensureParent(spec.Path.Parents())
	cmd := &cobra.Command{
		Use:    spec.Path.Name(),
		Short:  spec.Short,
		Long:   spec.Long,
		Hidden: spec.Hidden,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s requires a subcommand; see --help", cmd.Name())
		},
	}
	parent.AddCommand(cmd)
	key := strings.Join(spec.Path.Segments(), " ")
	r.parents[key] = cmd
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

// Register registers a runnable leaf command. It panics on any
// registration-time violation — these are programmer bugs that must be
// caught at startup.
func (r *Registry) Register(spec LeafSpec) {
	r.validateRegistration(spec)

	pathKey := strings.Join(spec.Path.Segments(), " ")
	r.ids[spec.ID] = true
	r.paths[pathKey] = true

	parent := r.ensureParent(spec.Path.Parents())

	cmd := &cobra.Command{
		Use:        spec.Path.Name(),
		Short:      spec.Doc.Short,
		Long:       spec.Doc.Long,
		Aliases:    spec.Doc.Aliases,
		Hidden:     spec.Doc.Hidden,
		Deprecated: spec.Doc.Deprecated,
		Args:       cobra.NoArgs,
		RunE:       r.buildRunE(spec),
	}

	if len(spec.Doc.Examples) > 0 {
		cmd.Example = strings.Join(spec.Doc.Examples, "\n")
	}

	addFlags(cmd, spec.Flags)
	parent.AddCommand(cmd)
}

// ---------------------------------------------------------------------------
// Registration-time validation (panics)
// ---------------------------------------------------------------------------

func (r *Registry) validateRegistration(spec LeafSpec) {
	if spec.Run == nil {
		panic("registry.Register: command " + spec.ID + " has nil Run")
	}

	pathKey := strings.Join(spec.Path.Segments(), " ")

	if r.ids[spec.ID] {
		panic("registry.Register: duplicate command ID: " + spec.ID)
	}
	if r.paths[pathKey] {
		panic("registry.Register: duplicate command path: " + pathKey)
	}

	// Check duplicate flag names.
	flagSet := make(map[string]bool, len(spec.Flags))
	for _, f := range spec.Flags {
		name := f.flagName()
		if flagSet[name] {
			panic("registry.Register: duplicate flag name: " + name)
		}
		flagSet[name] = true
	}

	// Required StringFlag with non-zero default.
	for _, f := range spec.Flags {
		if sf, ok := f.(StringFlag); ok && sf.Required && sf.Default != "" {
			panic("registry.Register: required StringFlag " + sf.Name + " must not have a non-zero default")
		}
		if sf, ok := f.(IntFlag); ok && sf.Required && sf.Default != 0 {
			panic("registry.Register: required IntFlag " + sf.Name + " must not have a non-zero default")
		}
	}

	// Check for duplicate short flags.
	shortSet := make(map[string]bool)
	for _, f := range spec.Flags {
		var short string
		switch fl := f.(type) {
		case StringFlag:
			short = fl.Short
		case IntFlag:
			short = fl.Short
		case BoolFlag:
			short = fl.Short
		case StringSliceFlag:
			short = fl.Short
		}
		if short != "" {
			if shortSet[short] {
				panic("registry.Register: duplicate short flag -" + short)
			}
			shortSet[short] = true
		}
	}

	// Validate rule groups.
	r.validateRuleGroups(spec.Rules, flagSet)
}

func (r *Registry) validateRuleGroups(rules RuleSpec, flagSet map[string]bool) {
	allExactly := make(map[string]bool)
	allAtMost := make(map[string]bool)

	for _, group := range rules.ExactlyOneOf {
		validateGroup("ExactlyOneOf", group, flagSet)
		for _, name := range group {
			allExactly[name] = true
		}
	}

	for _, group := range rules.AtMostOneOf {
		validateGroup("AtMostOneOf", group, flagSet)
		for _, name := range group {
			allAtMost[name] = true
		}
	}

	// Check for flags appearing in both ExactlyOneOf and AtMostOneOf.
	for name := range allExactly {
		if allAtMost[name] {
			panic("registry.Register: flag " + name + " appears in both ExactlyOneOf and AtMostOneOf")
		}
	}

	// Validate Requires.
	for source, targets := range rules.Requires {
		if !flagSet[source] {
			panic("registry.Register: Requires references unknown flag: " + source)
		}
		for _, t := range targets {
			if !flagSet[t] {
				panic("registry.Register: Requires references unknown flag: " + t)
			}
		}
	}
}

func validateGroup(kind string, group []string, flagSet map[string]bool) {
	if len(group) < 2 {
		panic("registry.Register: " + kind + " group must have at least 2 entries")
	}
	seen := make(map[string]bool, len(group))
	for _, name := range group {
		if !flagSet[name] {
			panic("registry.Register: " + kind + " references unknown flag: " + name)
		}
		if seen[name] {
			panic("registry.Register: " + kind + " contains duplicate flag: " + name)
		}
		seen[name] = true
	}
}

// ---------------------------------------------------------------------------
// addFlags
// ---------------------------------------------------------------------------

func addFlags(cmd *cobra.Command, flags []Flag) {
	for _, f := range flags {
		switch fl := f.(type) {
		case StringFlag:
			if fl.Short != "" {
				cmd.Flags().StringP(fl.Name, fl.Short, fl.Default, fl.Help)
			} else {
				cmd.Flags().String(fl.Name, fl.Default, fl.Help)
			}
		case IntFlag:
			if fl.Short != "" {
				cmd.Flags().IntP(fl.Name, fl.Short, fl.Default, fl.Help)
			} else {
				cmd.Flags().Int(fl.Name, fl.Default, fl.Help)
			}
		case BoolFlag:
			if fl.Short != "" {
				cmd.Flags().BoolP(fl.Name, fl.Short, fl.Default, fl.Help)
			} else {
				cmd.Flags().Bool(fl.Name, fl.Default, fl.Help)
			}
		case StringSliceFlag:
			if fl.Short != "" {
				cmd.Flags().StringSliceP(fl.Name, fl.Short, fl.Default, fl.Help)
			} else {
				cmd.Flags().StringSlice(fl.Name, fl.Default, fl.Help)
			}
		}

		if f.isRequired() {
			_ = cmd.MarkFlagRequired(f.flagName())
		}
		if f.isHidden() {
			_ = cmd.Flags().MarkHidden(f.flagName())
		}
	}
}

// ---------------------------------------------------------------------------
// buildRunE
// ---------------------------------------------------------------------------

func (r *Registry) buildRunE(spec LeafSpec) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		fv := NewFlagValues(cmd)

		// Runtime rule validation — returns error to cobra (exit 2).
		if err := validateRules(cmd, spec.Rules); err != nil {
			return err
		}

		// Reject empty strings for required StringFlags (cobra only
		// checks presence, not emptiness).
		if err := validateRequiredNonEmpty(fv, spec.Flags); err != nil {
			return err
		}

		// Enum validation.
		if err := validateEnums(fv, spec.Flags); err != nil {
			return err
		}

		// Custom validation callback.
		if spec.Validate != nil {
			if err := spec.Validate(fv); err != nil {
				return err
			}
		}

		// Execute based on mode.
		if spec.Exec.IsLocal() {
			result, err := spec.Run(cmd.Context(), nil, fv)
			if err != nil {
				r.printResult(spec.ID, errorJSON(err.Error()))
				return nil
			}
			r.printResult(spec.ID, result)
			return nil
		}

		// Bounded or Streaming: needs app.
		if err := r.initApp(); err != nil {
			r.printResult(spec.ID, errorJSON(err.Error()))
			return nil
		}
		defer r.closeApp()

		ctx, cancel := newContext(spec.Exec)
		defer cancel()

		result, err := spec.Run(ctx, r.app, fv)
		if err != nil {
			r.printResult(spec.ID, errorJSON(err.Error()))
			return nil
		}
		r.printResult(spec.ID, result)
		return nil
	}
}

// ---------------------------------------------------------------------------
// errorJSON
// ---------------------------------------------------------------------------

// errorJSON produces a properly-escaped JSON error envelope for stdout.
func errorJSON(msg string) string {
	escaped, _ := json.Marshal(msg)
	return fmt.Sprintf(`{"success":false,"data":null,"error":%s}`, escaped)
}

// ---------------------------------------------------------------------------
// resolveOutputMode
// ---------------------------------------------------------------------------

// resolveOutputMode converts the --output flag value to an OutputMode.
func (r *Registry) resolveOutputMode() output.OutputMode {
	switch r.formatFlag {
	case "human":
		return output.ModeHuman
	case "auto":
		if f, ok := r.writer.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
			return output.ModeHuman
		}
		return output.ModeJSON
	default:
		return output.ModeJSON
	}
}

// ---------------------------------------------------------------------------
// printResult
// ---------------------------------------------------------------------------

func (r *Registry) printResult(commandID, result string) {
	env, err := output.ParseEnvelope(result)
	if err != nil {
		// Parse failure: print raw result, exit 1.
		fmt.Fprintln(r.writer, result)
		r.exitCode = 1
		return
	}

	// Derive exit code from envelope.
	if !env.Success {
		r.exitCode = 1
	}

	mode := r.resolveOutputMode()

	if mode == output.ModeJSON {
		fmt.Fprintln(r.writer, result)
		return
	}

	// ModeHuman: try per-command formatter first.
	if formatted, ok, err := output.FormatHuman(commandID, env); ok {
		fmt.Fprintln(r.writer, formatted)
		return
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ %v\n", err)
	}

	// Try generic formatter.
	if formatted, err := output.GenericFormat(env); err == nil {
		fmt.Fprintln(r.writer, formatted)
		return
	}

	// Last resort: raw JSON.
	fmt.Fprintln(r.writer, result)
}

// ---------------------------------------------------------------------------
// initApp / closeApp
// ---------------------------------------------------------------------------

func (r *Registry) initApp() error {
	if r.app != nil {
		return nil
	}
	absStore, err := filepath.Abs(r.storeDir)
	if err != nil {
		return fmt.Errorf("invalid store path: %w", err)
	}
	r.app, err = commands.NewApp(absStore, r.version)
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}
	return nil
}

func (r *Registry) closeApp() {
	if r.app != nil {
		r.app.Close()
		r.app = nil
	}
}

// ---------------------------------------------------------------------------
// newContext
// ---------------------------------------------------------------------------

// newContext returns a context appropriate for the execution mode.
// Streaming: signal-based cancellation. Bounded: timeout (default 5 min).
// Local mode should not call this function.
func newContext(mode ExecMode) (context.Context, context.CancelFunc) {
	if mode.IsStreaming() {
		ctx, cancel := context.WithCancel(context.Background())
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			select {
			case <-sigChan:
				cancel()
			case <-ctx.Done():
			}
			signal.Stop(sigChan)
		}()
		return ctx, cancel
	}

	// Bounded.
	timeout := mode.Timeout()
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

// ---------------------------------------------------------------------------
// ensureParent
// ---------------------------------------------------------------------------

// ensureParent walks the parent segment list, creating intermediate parent
// commands as needed, and returns the final parent command.
func (r *Registry) ensureParent(parents []string) *cobra.Command {
	if len(parents) == 0 {
		return r.root
	}

	current := r.root
	for i := range parents {
		key := strings.Join(parents[:i+1], " ")
		if p, ok := r.parents[key]; ok {
			current = p
			continue
		}
		// Create an implicit parent.
		cmd := &cobra.Command{
			Use:   parents[i],
			Short: parents[i] + " commands",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("%s requires a subcommand; see --help", cmd.Name())
			},
		}
		current.AddCommand(cmd)
		r.parents[key] = cmd
		current = cmd
	}
	return current
}

// ---------------------------------------------------------------------------
// validateRules
// ---------------------------------------------------------------------------

// validateRules checks ExactlyOneOf, AtMostOneOf, and Requires at runtime.
func validateRules(cmd *cobra.Command, rules RuleSpec) error {
	for _, group := range rules.ExactlyOneOf {
		count := 0
		for _, name := range group {
			if cmd.Flags().Changed(name) {
				count++
			}
		}
		if count != 1 {
			return fmt.Errorf("exactly one of [%s] must be set", strings.Join(group, ", "))
		}
	}

	for _, group := range rules.AtMostOneOf {
		count := 0
		for _, name := range group {
			if cmd.Flags().Changed(name) {
				count++
			}
		}
		if count > 1 {
			return fmt.Errorf("at most one of [%s] may be set", strings.Join(group, ", "))
		}
	}

	for source, targets := range rules.Requires {
		if cmd.Flags().Changed(source) {
			for _, t := range targets {
				if !cmd.Flags().Changed(t) {
					return fmt.Errorf("flag --%s requires --%s", source, t)
				}
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// validateRequiredNonEmpty
// ---------------------------------------------------------------------------

// validateRequiredNonEmpty rejects empty strings for required StringFlags and
// empty slices for required StringSliceFlags. Cobra's MarkFlagRequired only
// checks that the flag was provided on the command line — it does not reject
// --flag="" or --flag=. This check turns those into usage errors (exit 2).
func validateRequiredNonEmpty(fv FlagValues, flags []Flag) error {
	for _, f := range flags {
		if !f.isRequired() {
			continue
		}
		switch fl := f.(type) {
		case StringFlag:
			if strings.TrimSpace(fv.String(fl.Name)) == "" {
				return fmt.Errorf("flag --%s requires a non-empty value", fl.Name)
			}
		case StringSliceFlag:
			vals := fv.StringSlice(fl.Name)
			if len(vals) == 0 {
				return fmt.Errorf("flag --%s requires at least one value", fl.Name)
			}
			allEmpty := true
			for _, v := range vals {
				if strings.TrimSpace(v) != "" {
					allEmpty = false
					break
				}
			}
			if allEmpty {
				return fmt.Errorf("flag --%s requires non-empty values", fl.Name)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// validateEnums
// ---------------------------------------------------------------------------

// validateEnums checks that StringFlag values are within their Enum set.
func validateEnums(fv FlagValues, flags []Flag) error {
	for _, f := range flags {
		sf, ok := f.(StringFlag)
		if !ok || len(sf.Enum) == 0 || !fv.IsSet(sf.Name) {
			continue
		}
		val := fv.String(sf.Name)
		found := false
		for _, allowed := range sf.Enum {
			if val == allowed {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("flag --%s must be one of [%s], got %q", sf.Name, strings.Join(sf.Enum, ", "), val)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Execute
// ---------------------------------------------------------------------------

// Execute runs the root command. It handles exit codes:
// exit 2 for cobra/usage errors, exit 1 for runtime errors.
func (r *Registry) Execute() {
	if err := r.root.Execute(); err != nil {
		fmt.Fprintln(r.writer, errorJSON(err.Error()))
		os.Exit(2)
	}
	if r.exitCode != 0 {
		os.Exit(r.exitCode)
	}
}
