# Tech Context: Warpbox

## 1. Development Environment

* **Local OS:** Windows (Primary debugging and testing environment).
* **Toolchain:** Go (Golang) latest stable version.
* **Execution:** Commands run locally via `go run` or compiled via `go build` for Windows testing.
* **IDE:** VS Code with Cline extension.
* When using curl, refer to "curl.exe" directly, as "curl" can be an alias for PowerShell's "Invoke-WebRequest".

## 2. Local debugging

* Although the CICD pipeline is set up, it takes some time to build and release, and then deploy into docker.
* For most iterative development, we should use a locally built .exe and rclone.exe to test things.

## 3. Build Targets & Cross-Compilation

* Go's native cross-compilation will be used to generate standalone executables.
* **Target Architectures:**`amd64` (x64), `386` (x86), `arm64`.
* **Target Operating Systems:**`windows`, `linux`, `darwin` (macOS).

## 4. CI/CD Pipeline

* **Platform:** GitHub Actions.
* **Workflow:** Automated linting, testing, and compilation triggered upon tagging a release.
* **Artefacts:** Standalone binaries outputted for all target OS/Architecture combinations (adopting a release structure similar to Zurg).
