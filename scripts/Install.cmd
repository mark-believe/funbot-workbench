@echo off
setlocal
set "WORKBENCH_DIR=%~dp0"
powershell.exe -NoProfile -Command "$ErrorActionPreference='Stop'; $d=$env:WORKBENCH_DIR; $exe=Join-Path $d 'workbench.exe'; if (!(Test-Path -LiteralPath $exe)) {throw 'Extract the entire ZIP first'}; $shell=New-Object -ComObject WScript.Shell; $link=$shell.CreateShortcut((Join-Path ([Environment]::GetFolderPath('DesktopDirectory')) 'MARKAI Workbench.lnk')); $link.TargetPath=$exe; $link.WorkingDirectory=$d; $link.IconLocation=(Join-Path $d 'workbench.ico'); $link.Save()"
if errorlevel 1 (pause & exit /b 1)
echo Desktop shortcut installed: MARKAI Workbench
pause
