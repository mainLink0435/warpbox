# Tech Context: Warpbox

## 1. Development Environment

* **Local OS:** Windows (Primary debugging and testing environment).
* **Toolchain:** Go (Golang) latest stable version.
* **Execution:** Commands run locally via `go run` or compiled via `go build` for Windows testing.
* **IDE:** VS Code with Cline extension.
* When using curl, refer to "curl.exe" directly, as "curl" can be an alias for PowerShell's "Invoke-WebRequest".

## 2. Build Targets & Cross-Compilation

* Go's native cross-compilation will be used to generate standalone executables.
* **Target Architectures:**`amd64` (x64), `386` (x86), `arm64`.
* **Target Operating Systems:**`windows`, `linux`, `darwin` (macOS).

## 3. CI/CD Pipeline (Future State)

* **Platform:** GitHub Actions.
* **Workflow:** Automated linting, testing, and compilation triggered upon tagging a release.
* **Artefacts:** Standalone binaries outputted for all target OS/Architecture combinations (adopting a release structure similar to Zurg).
