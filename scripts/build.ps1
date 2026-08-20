param(
    [string]$Output = "build\serial-gateway.exe"
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$commit = "unknown"
$buildID = "unknown"
if (Get-Command git -ErrorAction SilentlyContinue) {
    $safeRepoRoot = $repoRoot.Replace('\', '/')
    $candidate = git -c "safe.directory=$safeRepoRoot" -C $repoRoot rev-parse --short=12 HEAD 2>$null
    if ($LASTEXITCODE -eq 0) {
        $commit = $candidate.Trim()
        $buildID = $commit
        $workingTree = git -c "safe.directory=$safeRepoRoot" -C $repoRoot status --porcelain --untracked-files=normal 2>$null
        if ($LASTEXITCODE -eq 0 -and $workingTree) {
            $buildID += "-dirty"
        }
    }
}
$buildDate = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
$ldflags = "-s -w -X main.version=$buildID -X main.commit=$commit -X main.buildDate=$buildDate"
$outputPath = Join-Path $repoRoot $Output
$outputDirectory = Split-Path -Parent $outputPath

Push-Location $repoRoot
try {
    New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "go test failed with exit code $LASTEXITCODE" }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed with exit code $LASTEXITCODE" }
    go build -buildvcs=false -trimpath -ldflags $ldflags -o $outputPath ./cmd/serial-gateway
    if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
    Write-Output "Built $outputPath ($buildID, $buildDate)"
}
finally {
    Pop-Location
}
