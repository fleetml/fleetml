# Helpers for .github/workflows/windows-run.yml, dot-sourced by its PowerShell
# steps. Written for Windows PowerShell 5.1 (the runner's `powershell` shell)
# and kept ASCII: 5.1 reads a script without a byte order mark in the ANSI
# code page.

# The docs' $base for the release under test: releases/download/<tag> instead
# of releases/latest/download.
$ReleaseBase = "https://github.com/fleetml/fleetml/releases/download/$env:TAG"
$Notes = @()
$ExitCode = 0

# Add-Note <text>: a line for this step's row in the job summary. Every call
# rewrites the step's `note` output, so a step that fails keeps what it had.
function Add-Note([string]$Text) {
    $script:Notes += $Text
    "note=$($script:Notes -join '; ')" | Out-File -FilePath $env:GITHUB_OUTPUT -Encoding utf8 -Append
}

# Set-StepOutput <name> <value>
function Set-StepOutput([string]$Name, [string]$Value) {
    "$Name=$Value" | Out-File -FilePath $env:GITHUB_OUTPUT -Encoding utf8 -Append
}

# Test-Expect <what> <got> <ok>: prints the check as windows-test.md's
# "Expect" lines read, and fails the step when it does not hold.
function Test-Expect([string]$What, [string]$Got, [bool]$Ok) {
    Write-Host "Expect: $What"
    # Program output can span lines and start with a blank one (reg query
    # does): print it trimmed, continuation lines under the first.
    $lines = @($Got.Trim() -split "\r?\n")
    Write-Host "Got:    $($lines[0])"
    $lines | Select-Object -Skip 1 | ForEach-Object { Write-Host "        $_" }
    if (-not $Ok) {
        Add-Note "FAILED: $What"
        throw "FAILED: $What"
    }
    Write-Host 'PASS'
}

# Invoke-NativeAllowFail <program> <arguments...>: runs a program, prints the
# command and its output (stdout and stderr), and returns the output as one
# string; the exit code is in $ExitCode. LASTEXITCODE is reset, since the
# runner fails a step whose last program exited non-zero.
function Invoke-NativeAllowFail {
    $exe = $args[0]
    $rest = @($args | Select-Object -Skip 1)
    Write-Host "> $exe $rest"
    $ErrorActionPreference = 'Continue'
    $out = @(& $exe @rest 2>&1 | ForEach-Object { "$_" })
    $script:ExitCode = $LASTEXITCODE
    $global:LASTEXITCODE = 0
    $out | ForEach-Object { Write-Host "  $_" }
    $out -join "`n"
}

# Invoke-Native <program> <arguments...>: the same, failing the step on a
# non-zero exit.
function Invoke-Native {
    $out = Invoke-NativeAllowFail @args
    if ($script:ExitCode -ne 0) { throw "$($args[0]) exited with $script:ExitCode" }
    $out
}

# Start-FakeServer: starts clitest-server (built by the "fake" step into
# $env:RUNNER_TEMP) on 127.0.0.1:18081 with $env:CLITEST_KEY and waits for
# /healthz. Each step that drives the CLI starts its own: on the Windows
# runner a process started by one step does not survive into the next (the
# first run's calls step found the port refused after the fake step had
# passed its health check).
function Start-FakeServer {
    $server = "$env:RUNNER_TEMP\clitest-server.exe"
    if (-not (Test-Path $server)) {
        $null = Invoke-Native go build -tags clitest -o $server ./internal/cli/clitest/cmd/clitest-server
    }
    $p = Start-Process $server -ArgumentList '-addr', '127.0.0.1:18081', '-key', $env:CLITEST_KEY -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput "$env:RUNNER_TEMP\clitest-server.log" -RedirectStandardError "$env:RUNNER_TEMP\clitest-server.err"
    $health = $null
    foreach ($i in 1..30) {
        try { $health = Invoke-RestMethod http://127.0.0.1:18081/healthz -UseBasicParsing; break } catch { Start-Sleep 1 }
    }
    if ($health.status -ne 'ok') {
        if (Test-Path "$env:RUNNER_TEMP\clitest-server.err") { Get-Content "$env:RUNNER_TEMP\clitest-server.err" | ForEach-Object { Write-Host "  $_" } }
        throw 'clitest-server did not answer /healthz on 127.0.0.1:18081'
    }
    Write-Host "clitest-server running (PID $($p.Id)) on 127.0.0.1:18081"
    $p
}

# Save-ReleaseAsset <name> <file>: downloads an asset of release $env:TAG.
# Public repository: the docs' URL, $ReleaseBase/<name>. In a private
# repository that URL answers 404, so the asset is looked up through the API
# and downloaded with the job's token.
function Save-ReleaseAsset([string]$Name, [string]$OutFile) {
    if ($env:REPO_PRIVATE -ne 'true') {
        Invoke-WebRequest "$ReleaseBase/$Name" -OutFile $OutFile -UseBasicParsing
        return
    }
    $headers = @{ Authorization = "Bearer $env:GH_TOKEN"; 'X-GitHub-Api-Version' = '2022-11-28' }
    $release = Invoke-RestMethod "https://api.github.com/repos/$env:GITHUB_REPOSITORY/releases/tags/$env:TAG" -Headers $headers -UseBasicParsing
    $asset = $release.assets | Where-Object { $_.name -eq $Name }
    if (-not $asset) { throw "release $env:TAG has no asset $Name" }
    Write-Host "NOTE: private repository: $Name is downloaded as asset $($asset.id) through the GitHub API with the job's token; the docs' public URL is $ReleaseBase/$Name"
    $headers.Accept = 'application/octet-stream'
    Invoke-WebRequest $asset.url -Headers $headers -OutFile $OutFile -UseBasicParsing
}

# Get-PEMachine <file>: the architecture a Windows executable is built for.
function Get-PEMachine([string]$Path) {
    $bytes = [IO.File]::ReadAllBytes($Path)
    $pe = [BitConverter]::ToInt32($bytes, 0x3c)
    switch ([BitConverter]::ToUInt16($bytes, $pe + 4)) {
        0x8664 { 'amd64' }
        0xAA64 { 'arm64' }
        default { 'other' }
    }
}

# Update-SessionPath: a new PowerShell window takes Path from the machine and
# user values in the registry, while the runner starts every step with its
# own environment; this gives the step the Path a new window would have.
function Update-SessionPath {
    $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User')
}

# Write-JobSummary <title> <heading> <rows>: the job summary's table. Each row
# is "<step id>=<label>"; the result and the note come from $env:STEPS, the
# job's steps context as JSON.
function Write-JobSummary([string]$Title, [string]$Heading, [string[]]$Rows) {
    $steps = $env:STEPS | ConvertFrom-Json
    $md = @("### $Title", '', $Heading, '', '| Step | Result | Note |', '|---|---|---|')
    foreach ($row in $Rows) {
        $id, $label = $row -split '=', 2
        $step = $steps.$id
        $result = switch ($step.outcome) {
            'success' { 'pass' }
            'failure' { '**FAIL**' }
            'skipped' { 'skipped' }
            $null { 'not run' }
            default { $step.outcome }
        }
        $note = "$($step.outputs.note)" -replace '\|', '\|'
        $md += "| $label | $result | $note |"
    }
    $md | Out-File -FilePath $env:GITHUB_STEP_SUMMARY -Encoding utf8 -Append
    $md | ForEach-Object { Write-Host $_ }
}
