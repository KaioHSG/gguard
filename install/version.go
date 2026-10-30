package install

// Version is the release version of every GopherGuard binary.
//
// It is the single source of truth: the app and the installer print it and
// build.go stamps it into the Windows resources of all three executables. This
// is the only place to bump.
var Version = "0.2.4"
