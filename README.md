# DeepSeek Harness PC

Janela nativa do Windows para o [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) — sem browser, sem barra de endereço, com ícone próprio e modelos free do OpenCode.

![DeepSeek Harness](assets/deepseek-500.png)

## Instalar

Cole no PowerShell:

```powershell
irm https://raw.githubusercontent.com/Ryanabcraft/deepseek-harness-pc/main/install.ps1 | iex
```

O instalador faz, em ordem, e **pula o que já existe**:

1. **Node.js LTS** — se não houver `node`, instala via winget.
2. **DSH** — se não houver `dsh`, roda `npm i -g @deepseek-ai/dsh`.
3. **OpenCode CLI** — opcional, pergunta antes. O bridge de modelos free **não precisa** dele; fala direto com `opencode.ai/inference/v1`.
4. **App** — baixa o `DeepSeek-Harness.exe` do último release, instala em `%LOCALAPPDATA%\Programs\DeepSeekHarness` e cria atalho no Desktop.

Flags: `-Yes` (não pergunta nada, instala o OpenCode) e `-NoShortcut`.

## O que o app faz

- Splash próprio e janela **WebView2** (sem navegador).
- Roda `dsh web --no-open` oculto (`CREATE_NO_WINDOW`) e navega para a URL com token quando ela aparece.
- Porta `3080` ocupada → sobe em porta livre automaticamente.
- Job Object `KILL_ON_JOB_CLOSE`: fecha a janela e o processo `node` do dsh morre junto.
- Argumentos extras são repassados: `DeepSeek Harness.exe --port 8080`.

## Build a partir do fonte

```powershell
git clone https://github.com/Ryanabcraft/deepseek-harness-pc
cd deepseek-harness-pc
$env:CGO_ENABLED=1
$env:CC="C:\msys64\mingw64\bin\gcc.exe"
go build -trimpath -ldflags "-s -w -H=windowsgui" -o "DeepSeek Harness.exe" .
```

Requisitos: Go 1.22+, MinGW-w64 (CGO) e WebView2 Runtime (já vem no Windows 11).

O ícone é gerado por `go run ./tools/genicon` (baleia → `icon.ico` multi-resolução) e embutido via `windres` (`app.rc` → `rsrc.syso`).

## Aviso

Projeto não oficial, sem vínculo com DeepSeek ou OpenCode. O bridge usa a API pública de modelos free do OpenCode, que pode mudar ou limitar o uso sem aviso. Não envie conteúdo sensível.
