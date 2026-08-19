param(
    [string]$Output = "serial-gateway.exe",
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$commit = "unknown"
if (Get-Command git -ErrorAction SilentlyContinue) {
    $candidate = git -C $repoRoot rev-parse --short HEAD 2>$null
    if ($LASTEXITCODE -eq 0) {
        $commit = $candidate.Trim()
    }
}
$buildDate = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
$ldflags = "-s -w -X main.version=$Version -X main.commit=$commit -X main.buildDate=$buildDate"
$outputPath = Join-Path $repoRoot $Output

Push-Location $repoRoot
try {
    go test ./...
    go vet ./...
    go build -trimpath -ldflags $ldflags -o $outputPath ./cmd/serial-gateway
    & $outputPath -version
}
finally {
    Pop-Location
}
