param(
  [string]$GoExecutable = 'go',
  [string]$OutputDir = (Join-Path $PSScriptRoot '..\dist')
)

$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$destination = [System.IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Path $destination -Force | Out-Null
$stage = Join-Path $destination ('stage-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage | Out-Null

try {
  Push-Location $repo
  try {
    & $GoExecutable test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    & $GoExecutable build -trimpath -buildvcs=false -o (Join-Path $stage 'workbench.exe') .
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
  } finally {
    Pop-Location
  }

  foreach ($file in @('README.md', 'LICENSE', 'NOTICE.md', 'DEPLOYMENT.md')) {
    Copy-Item -LiteralPath (Join-Path $repo $file) -Destination $stage
  }
  foreach ($folder in @('福泽模拟', '扩组核算')) {
    Copy-Item -LiteralPath (Join-Path $repo $folder) -Destination $stage -Recurse
  }

  $asset = 'MARKAI-Funbot-Workbench-Windows-x64.zip'
  $archive = Join-Path $destination $asset
  if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive -Force }
  [System.IO.Compression.ZipFile]::CreateFromDirectory(
    $stage, $archive, [System.IO.Compression.CompressionLevel]::Optimal,
    $false, [System.Text.Encoding]::UTF8
  )
  $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
  Set-Content -LiteralPath (Join-Path $destination 'SHA256SUMS.txt') -Value "$hash  $asset" -Encoding ascii
  Write-Output $archive
  Write-Output "SHA-256 $hash"
} finally {
  $resolvedStage = [System.IO.Path]::GetFullPath($stage)
  if ($resolvedStage.StartsWith($destination + [System.IO.Path]::DirectorySeparatorChar, [System.StringComparison]::OrdinalIgnoreCase)) {
    Remove-Item -LiteralPath $resolvedStage -Recurse -Force
  }
}
