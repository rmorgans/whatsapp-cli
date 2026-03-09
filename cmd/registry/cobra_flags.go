package registry

import "github.com/spf13/cobra"

// cobraFlags adapts a cobra command's flag set to FlagValues.
type cobraFlags struct {
	cmd *cobra.Command
}

// NewFlagValues creates a FlagValues from a cobra command.
func NewFlagValues(cmd *cobra.Command) FlagValues {
	return &cobraFlags{cmd: cmd}
}

func (f *cobraFlags) String(name string) string {
	v, _ := f.cmd.Flags().GetString(name)
	return v
}

func (f *cobraFlags) Int(name string) int {
	v, _ := f.cmd.Flags().GetInt(name)
	return v
}

func (f *cobraFlags) Bool(name string) bool {
	v, _ := f.cmd.Flags().GetBool(name)
	return v
}

func (f *cobraFlags) StringSlice(name string) []string {
	v, _ := f.cmd.Flags().GetStringSlice(name)
	return v
}

func (f *cobraFlags) IsSet(name string) bool {
	return f.cmd.Flags().Changed(name)
}
