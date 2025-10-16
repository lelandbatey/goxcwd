
# goxcwd

`goxcwd` is a re-implementation of `xcwd` but in Go. When run, it reads the currently in-focus X11
window and writes it current working directory (a path) to stdout, followed by a newline.

Additionally, excludes certain executables when traversing child processes (via an easy to edit list
at the top of file). These are excluded because they're typically badly-behaved deep child processes
which will set their current working directory far outside of the current working directory of the
original parent. For example, when I have vim open in my terminal and I'm editing Go files, vim will
run `gopls` and `gopls` will frequently sit in the `~/.config/go/telemetry/local` directory, causing
vanilla `xcwd` to report my current directory as `~/.config/go/telemetry/local`. This is super
annoying, so we block certain executables like `gopls`. You can add your own exectables to the list
to be left out at build time.
