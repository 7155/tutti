[CmdletBinding()]
param(
  [string]$UserHome = "",
  [switch]$SkipOpenCode,
  [switch]$SkipHermes,
  [switch]$SkipKimi
)

$ErrorActionPreference = "Stop"

function Write-Utf8NoBom([string]$Path, [string]$Content) {
  $parent = Split-Path -Parent $Path
  New-Item -ItemType Directory -Force -Path $parent | Out-Null
  [System.IO.File]::WriteAllText(
    $Path,
    $Content,
    [System.Text.UTF8Encoding]::new($false)
  )
}

function Backup-IfPresent([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path)) {
    return
  }
  $backup = "$Path.bak-$(Get-Date -Format yyyyMMdd-HHmmss)"
  Copy-Item -LiteralPath $Path -Destination $backup -Force
  Write-Host "[ok] backed up $Path -> $backup"
}

function TomlString([string]$Value) {
  return '"' + $Value.Replace('\', '\\').Replace('"', '\"') + '"'
}

function YamlString([string]$Value) {
  return "'" + $Value.Replace("'", "''") + "'"
}

if (-not $UserHome) {
  $UserHome = [Environment]::GetFolderPath("UserProfile")
}
$claudeSettingsPath = Join-Path $UserHome ".claude\settings.json"
if (-not (Test-Path -LiteralPath $claudeSettingsPath)) {
  throw "Claude settings file was not found: $claudeSettingsPath"
}

$settings = Get-Content -Raw -LiteralPath $claudeSettingsPath | ConvertFrom-Json
$envSettings = $settings.env
$token = [string]$envSettings.ANTHROPIC_AUTH_TOKEN
$baseUrl = [string]$envSettings.ANTHROPIC_BASE_URL
$model = [string]$envSettings.ANTHROPIC_DEFAULT_SONNET_MODEL
if (-not $model) {
  $model = [string]$envSettings.ANTHROPIC_DEFAULT_OPUS_MODEL
}
if (-not $model) {
  $model = "glm-5"
}

if (-not $token) {
  throw "ANTHROPIC_AUTH_TOKEN is missing from $claudeSettingsPath"
}
if ($baseUrl -notmatch '^https://') {
  throw "Refusing to write a non-HTTPS Anthropic base URL: $baseUrl"
}

if (-not $SkipHermes) {
  $hermesHome = Join-Path $UserHome ".hermes"
  $hermesEnvPath = Join-Path $hermesHome ".env"
  $hermesConfigPath = Join-Path $hermesHome "config.yaml"
  Backup-IfPresent $hermesEnvPath
  Backup-IfPresent $hermesConfigPath
  Write-Utf8NoBom $hermesEnvPath (@(
    "ANTHROPIC_API_KEY=$token"
    "ANTHROPIC_BASE_URL=$baseUrl"
    "HERMES_INFERENCE_MODEL=$model"
  ) -join "`n")
  Write-Utf8NoBom $hermesConfigPath (@(
    "model:"
    "  provider: custom"
    "  default: $(YamlString $model)"
    "  base_url: $(YamlString $baseUrl)"
    "  api_key: $(YamlString $token)"
  ) -join "`n")
  Write-Host "[ok] configured Hermes Anthropic-compatible endpoint"
}

if (-not $SkipKimi) {
  $kimiHome = Join-Path $UserHome ".kimi-code"
  $kimiConfigPath = Join-Path $kimiHome "config.toml"
  Backup-IfPresent $kimiConfigPath
  Write-Utf8NoBom $kimiConfigPath (@(
    "[providers.anthropic]"
    "type = $(TomlString 'anthropic')"
    "base_url = $(TomlString $baseUrl)"
    "api_key = $(TomlString $token)"
    ""
    "[models.$(TomlString $model)]"
    "provider = $(TomlString 'anthropic')"
    "model = $(TomlString $model)"
    "max_context_size = 200000"
    ""
    "default_model = $(TomlString $model)"
  ) -join "`n")
  Write-Host "[ok] configured Kimi Code Anthropic provider"
}

if (-not $SkipOpenCode) {
  $openCodeHome = Join-Path $UserHome ".config\opencode"
  $openCodeConfigPath = Join-Path $openCodeHome "opencode.jsonc"
  Backup-IfPresent $openCodeConfigPath
  $openCodeConfig = [ordered]@{
    '$schema' = "https://opencode.ai/config.json"
    model = "tutti-anthropic/$model"
    provider = [ordered]@{
      'tutti-anthropic' = [ordered]@{
        npm = "@ai-sdk/anthropic"
        name = "Tutti Anthropic"
        options = [ordered]@{
          baseURL = $baseUrl
          apiKey = $token
        }
        models = [ordered]@{
          $model = [ordered]@{
            name = $model
            limit = [ordered]@{
              context = 200000
              output = 65536
            }
          }
        }
      }
    }
  }
  Write-Utf8NoBom $openCodeConfigPath (($openCodeConfig | ConvertTo-Json -Depth 10) + "`n")
  Write-Host "[ok] configured OpenCode Anthropic-compatible endpoint"
}

Write-Host "[ok] provider configuration completed without printing credentials"
