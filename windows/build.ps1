param(
    [ValidateSet("win-x64", "win-arm64")]
    [string]$Runtime = "win-x64"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Project = Join-Path $Root "src/FastCopy.Windows/FastCopy.Windows.csproj"
$Tests = Join-Path $Root "tests/FastCopy.Core.SmokeTests/FastCopy.Core.SmokeTests.csproj"
$PublishDirectory = Join-Path $Root "dist/clipboard-assistant-$Runtime"
$Archive = Join-Path $Root "dist/ClipboardAssistant-windows-$Runtime-v0.1.1.zip"

dotnet run --project $Tests --configuration Release

if (Test-Path $PublishDirectory) {
    Remove-Item $PublishDirectory -Recurse -Force
}

dotnet publish $Project `
    --configuration Release `
    --runtime $Runtime `
    --self-contained true `
    --output $PublishDirectory `
    -p:PublishSingleFile=true `
    -p:IncludeNativeLibrariesForSelfExtract=true `
    -p:EnableCompressionInSingleFile=true `
    -p:PublishTrimmed=false `
    -p:DebugType=None `
    -p:DebugSymbols=false

if (Test-Path $Archive) {
    Remove-Item $Archive -Force
}
Compress-Archive -Path (Join-Path $PublishDirectory "*") -DestinationPath $Archive

Write-Host "Published: $PublishDirectory"
Write-Host "Archive:   $Archive"
