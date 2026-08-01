# forkashi build targets.
GO := go
# Lazy (=, not :=): only shells out to xcrun when `apple` actually runs, so
# `make build` stays quiet on Linux/non-macOS where xcrun doesn't exist.
SWIFT_LIB = $(shell xcrun --show-sdk-path)/usr/lib/swift

# Default: pure-Go, cross-platform, no cgo, no Apple backend.
# Requires Go 1.25+ on your PATH (check with `go version`); on Linux, most
# distro packages lag behind — see README.md § Install for a manual install.
.PHONY: build
build:
	$(GO) build -o forkashi .

# Apple build: NSSpellChecker + Foundation Models. Requires macOS + Xcode (cgo + swiftc).
# Compiles the Swift FM bridge to a static lib, then builds with the applegrammar tag.
# NOTE (forkashi): newGrammarChecker is wired to Grammalecte unconditionally
# (see grammar_backend.go), so this tag no longer switches the active backend —
# kept for reference/future use, not currently load-bearing in this fork.
.PHONY: apple
apple: libokashifm.a
	CGO_ENABLED=1 CGO_LDFLAGS="-L$(SWIFT_LIB)" $(GO) build -tags applegrammar -o forkashi-apple .

libokashifm.a: grammar_apple_fm.swift
	xcrun swiftc -emit-library -static -o libokashifm.a grammar_apple_fm.swift -framework FoundationModels

# Record the README demo GIF from demo.tape. Requires VHS (brew install vhs).
.PHONY: demo
demo:
	vhs demo.tape

.PHONY: clean
clean:
	rm -f forkashi forkashi-apple libokashifm.a
