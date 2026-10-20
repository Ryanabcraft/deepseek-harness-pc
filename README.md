# DeepSeek Harness PC

Janela nativa do Windows para o [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) — sem browser, sem barra de endereço, com ícone próprio e modelos free do OpenCode.

![DeepSeek Harness](assets/deepseek-500.png)

## Instalar

**Instalador gráfico (recomendado)** — baixa e abre:

**[DeepSeek-Harness-Setup.exe](https://github.com/Ryanabcraft/deepseek-harness-pc/releases/latest/download/DeepSeek-Harness-Setup.exe)**

Escolhe idioma (English / Português), mostra cada etapa com progresso e instala o atalho no Desktop.

> Os binários não têm assinatura digital. Se o SmartScreen ou o antivírus reclamar, é falso positivo — veja [Falsos positivos de antivírus](#falsos-positivos-de-antivírus) e o código-fonte completo no repositório.

**Ou por linha de comando:**

```powershell
irm https://raw.githubusercontent.com/Ryanabcraft/deepseek-harness-pc/main/install.ps1 | iex
```

O setup pede UAC ao abrir (manifest `requireAdministrator`) e instala em `C:\Program Files\DeepSeek Harness`, com atalho no Desktop e no Menu Iniciar, entrada em **Aplicativos instalados** e desinstalador próprio. Ao clicar em **Abrir**, o setup fecha sozinho. Nenhum console/PowerShell aparece — tudo roda oculto.

O instalador faz, em ordem, e **pula o que já existe**:

1. **Node.js LTS** — se não houver `node`, instala via winget.
2. **DSH** — se não houver `dsh`, roda `npm i -g @deepseek-ai/dsh`.
3. **OpenCode bridge** — instala o plugin `dsh-opencode-bridge@0.5.2` no perfil `web` (é ele que fornece os modelos free; sem isso o provider `opencode-bridge` não existe).
4. **OpenCode CLI** — opcional, pergunta antes. O bridge **não precisa** dele; fala direto com `opencode.ai/inference/v1`.
5. **App** — baixa o `DeepSeek-Harness.exe` do último release, instala em `%LOCALAPPDATA%\Programs\DeepSeekHarness` e cria atalho no Desktop.

Flags: `-Yes` (não pergunta nada, instala o OpenCode) e `-NoShortcut`.

## Desinstalar

Pelo **Aplicativos instalados** do Windows, pelo atalho `DeepSeek-Harness-Uninstall.exe` na pasta de instalação, ou:

```powershell
& "C:\Program Files\DeepSeek Harness\DeepSeek-Harness-Uninstall.exe"
```

O desinstalador tem só três itens: **DeepSeek Harness** (marcado por padrão) remove o app, o `dsh`, a ponte OpenCode e os dados em `~/.dsh` — irreversível. **OpenCode CLI** e **Node.js** ficam como opcionais, desmarcados, porque são compartilhados com outros projetos.

## O que o app faz

- **Na primeira execução** escolhe sozinho o primeiro modelo free disponível do catálogo e grava em `~/.dsh/profiles/<perfil>/cordis.patch.yml` (`agent-default-model`). Ordem de preferência:
  `deepseek-v4.1-flash-free` → `minimax-m3.1-flash-free` → `mimo-v2.6-flash-free` → `glm-5.3-flash-free` → `nemotron-3.5-lightning-free` → `longcat-2.5-preview-free` → `beam-free` → `big-pickle` → `nemotron-3.5-super-vl-free`.
  Depois disso nunca mais mexe — se o modelo sumir, a escolha continua. Faz backup `.bak-model` e escrita atômica.
- Splash próprio e janela **WebView2** (sem navegador).
- Roda `dsh web --no-open` oculto (`CREATE_NO_WINDOW`) e navega para a URL com token quando ela aparece.
- Porta `3080` ocupada → sobe em porta livre automaticamente.
- Job Object `KILL_ON_JOB_CLOSE`: fecha a janela e o processo `node` do dsh morre junto.
- Argumentos extras são repassados: `DeepSeek Harness.exe --port 8080`.

## Falsos positivos de antivírus

Os três executáveis (app, setup e desinstalador) **não são assinados digitalmente**. O projeto não possui certificado de _code signing_, então o Windows SmartScreen pode exibir "editor desconhecido" e alguns antivírus podem classificar os arquivos como suspeitos. Isso é esperado para binários novos e sem assinatura e não indica comprometimento.

O código-fonte completo está neste repositório, sem ofuscação, sem empacotamento e sem componente fechado. O comportamento que costuma chamar a atenção de um scanner é o esperado de um instalador:

- baixa arquivos do GitHub (Node.js e o próprio app);
- executa `npm` e `winget` em segundo plano, com a janela de console oculta;
- cria atalhos no Desktop/Menu Iniciar e uma entrada no registro do Windows;
- abre uma janela WebView2 e acessa `opencode.ai/inference/v1`.

Para verificar por conta própria:

1. Clone o repositório e compile com os comandos da seção abaixo.
2. Compare o SHA-256 do binário gerado com o do asset publicado:

```powershell
Get-FileHash ".\DeepSeek Harness.exe" -Algorithm SHA256
```

3. Se preferir, rode o app direto do código, sem baixar executável nenhum.

Cada release é gerado a partir do commit da tag correspondente, e todo o histórico de alterações fica visível no repositório.

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
