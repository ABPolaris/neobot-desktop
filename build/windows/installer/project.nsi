Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the ProjectInfo file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "MyProject" # Default "{{.Name}}"
## !define INFO_COMPANYNAME    "MyCompany" # Default "{{.Info.CompanyName}}"
## !define INFO_PRODUCTNAME    "MyProduct" # Default "{{.Info.ProductName}}"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "{{.Info.ProductVersion}}"
## !define INFO_COPYRIGHT      "Copyright" # Default "{{.Info.Copyright}}"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

## 语言：先列中文，MUI 会按系统语言自动选择；系统语言不匹配时回落到第一个。
## SimpChinese 由 NSIS 自带的语言文件提供，向导按钮/提示会自动变成中文。
!insertmacro MUI_LANGUAGE "SimpChinese" # 简体中文（主语言）
!insertmacro MUI_LANGUAGE "English"     # English fallback

## Code signing hooks. They stay commented so a machine without a certificate
## can still build; CI signs the produced binaries instead (see qegg-release.yml).
## To sign from the build machine, uncomment both lines and put signtool on PATH.
#!uninstfinalize 'signtool sign /fd SHA256 /td SHA256 /tr http://timestamp.digicert.com "%1"'
#!finalize 'signtool sign /fd SHA256 /td SHA256 /tr http://timestamp.digicert.com "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.

## Install directory memory ---------------------------------------------------
## wails_tools.nsh is regenerated on every "wails build", so customisations must
## live here. NSIS InstallDirRegKey is not an option: it only reads the registry
## (it never writes it) and it runs with the 32-bit registry view, while
## wails_tools.nsh writes the uninstall key through SetRegView 64.
## LogicLib (${If}) and FileFunc (${GetParent}) come from wails_tools.nsh/MUI.nsh,
## which this file already relies on for ${UNINST_KEY} and the other wails.* macros.

!ifdef WAILS_INSTALL_SCOPE
  !if "${WAILS_INSTALL_SCOPE}" == "user"
    !define NEOBOT_UNINST_ROOT     HKCU
    !define NEOBOT_DEFAULT_INSTDIR "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
  !else
    !define NEOBOT_UNINST_ROOT     HKLM
    !define NEOBOT_DEFAULT_INSTDIR "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
  !endif
!else
  !define NEOBOT_UNINST_ROOT       HKLM
  !define NEOBOT_DEFAULT_INSTDIR   "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif

InstallDir "${NEOBOT_DEFAULT_INSTDIR}"

# Install directory recorded by a previous run; empty when there is none.
Var NeoBotPrevDir

# Reads $NeoBotPrevDir. InstallLocation is written by this script; DisplayIcon and
# UninstallString are read for installs made before that value existed.
!macro NeoBotReadPrevInstallDir
    StrCpy $NeoBotPrevDir ""
    SetRegView 64

    ReadRegStr $NeoBotPrevDir ${NEOBOT_UNINST_ROOT} "${UNINST_KEY}" "InstallLocation"

    ${If} $NeoBotPrevDir == ""
        ReadRegStr $0 ${NEOBOT_UNINST_ROOT} "${UNINST_KEY}" "DisplayIcon"
        ${If} $0 != ""
            ${GetParent} "$0" $NeoBotPrevDir
        ${Else}
            ReadRegStr $0 ${NEOBOT_UNINST_ROOT} "${UNINST_KEY}" "UninstallString"
            ${If} $0 != ""
                StrCpy $0 $0 "" 1
                StrCpy $0 $0 -1
                ${GetParent} "$0" $NeoBotPrevDir
            ${EndIf}
        ${EndIf}
    ${EndIf}

    SetRegView lastused
!macroend

ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture

   # Reuse the previous install directory unless /D= or InstallDir already moved it.
   !insertmacro NeoBotReadPrevInstallDir
   ${If} $NeoBotPrevDir != ""
   ${AndIf} $INSTDIR == "${NEOBOT_DEFAULT_INSTDIR}"
   ${AndIf} ${FileExists} "$NeoBotPrevDir\"
       StrCpy $INSTDIR $NeoBotPrevDir
   ${EndIf}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller

    # Remember where this copy lives so the next run can pre-fill the directory page
    # and "Apps & features" can show the location. wails.writeUninstaller already set
    # the 64-bit view; set it again so this block stays correct if that macro changes.
    SetRegView 64
    WriteRegStr ${NEOBOT_UNINST_ROOT} "${UNINST_KEY}" "InstallLocation" "$INSTDIR"

    # Installing somewhere else leaves the old folder behind: report it, never delete it.
    ${If} $NeoBotPrevDir != ""
    ${AndIf} $NeoBotPrevDir != $INSTDIR
    ${AndIf} ${FileExists} "$NeoBotPrevDir\${PRODUCT_EXECUTABLE}"
        ${IfNot} ${Silent}
            MessageBox MB_OK|MB_ICONINFORMATION "An older copy is still installed in:$\r$\n$NeoBotPrevDir$\r$\n$\r$\nThis setup installs to a new folder. The old folder was left untouched; delete it manually if it is no longer needed."
        ${EndIf}
    ${EndIf}
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    # Drop the autostart entry: otherwise Windows keeps launching an executable
    # that no longer exists. The launcher is a 64-bit process and writes this value
    # in the 64-bit view, while the installer runs as 32-bit.
    SetRegView 64
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "NeoBot Desktop"

    # The launcher keeps running in the tray, so uninstalling while it is up would
    # leave a half-deleted folder with no uninstall entry left behind.
    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        ClearErrors
        Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
        ${If} ${Errors}
            ${IfNot} ${Silent}
                MessageBox MB_OK|MB_ICONEXCLAMATION "NeoBot Desktop is still running. Quit it from the tray icon first, then run the uninstaller again."
            ${EndIf}
            Abort
        ${EndIf}
    ${EndIf}

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
