# DeepSeek Harness PC

Janela nativa do Windows para o [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) — sem browser, sem barra de endereço, com ícone próprio e modelos free do OpenCode.

![DeepSeek Harness](assets/deepseek-500.png)

## Instalar

**Instalador gráfico (recomendado)** — baixa e abre:

**[DeepSeek-Harness-Setup.exe](https://github.com/Ryanabcraft/deepseek-harness-pc/releases/latest/download/DeepSeek-Harness-Setup.exe)**

Escolhe idioma (English / Português), mostra cada etapa com progresso e instala o atalho no Desktop.

**Ou por linha de comando:**

```powershell
irm https://raw.githubusercontent.com/Ryanabcraft/deepseek-harness-pc/main/install.ps1 | iex
```

O setup pede UAC ao abrir (manifest `requireAdministrator`) e instala em `C:\Program Files\DeepSeek Harness`, com atalho no Desktop e no Menu Iniciar, entrada em **Aplicativos instalados** e desinstalador próprio. Ao clicar em **Abrir**, o setup fecha sozinho. Nenhum console/PowerShell aparece — tudo roda oculto.

O instalador faz, em ordem, e **pula o que já existe**:

1. **Node.js LTS** — se não houver `node`, instala via winget.
2. **DSH** — se não houver `dsh`, roda `npm i -g @deepseek-ai/dsh`.
3. **OpenCode CLI** — opcional, pergunta antes. O bridge de modelos free **não precisa** dele; fala direto com `opencode.ai/inference/v1`.
4. **App** — baixa o `DeepSeek-Harness.exe` do último release, instala em `%LOCALAPPDATA%\Programs\DeepSeekHarness` e cria atalho no Desktop.

Flags: `-Yes` (não pergunta nada, instala o OpenCode) e `-NoShortcut`.

## Desinstalar

Pelo **Aplicativos instalados** do Windows, pelo atalho `DeepSeek-Harness-Uninstall.exe` na pasta de instalação, ou:

```powershell
& "C:\Program Files\DeepSeek Harness\DeepSeek-Harness-Uninstall.exe"
```

O desinstalador lista o que está instalado e você marca só o que quer remover: app, `dsh`, `opencode`, Node.js (desmarcado por padrão) e dados do DSH em `~/.dsh` (desmarcado, irreversível).

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
