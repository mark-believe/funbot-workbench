@echo off
setlocal
set "WORKBENCH_DIR=%~dp0"
powershell.exe -NoProfile -STA -Command "$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms; $p=New-Object Windows.Forms.FolderBrowserDialog; $p.Description='Select the OLD workbench folder containing .data'; if ($p.ShowDialog() -eq 'OK') { & (Join-Path $env:WORKBENCH_DIR 'workbench.exe') -migrate-from $p.SelectedPath; exit $LASTEXITCODE }"
pause
