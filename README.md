# BruhaNet bot

## Установка / обновление

```bash
curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | bash
```

Сразу указать каталог (по умолчанию `/opt/bruhanet`):

```bash
curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | BRUHANET_DIR=/opt/bruhanet bash
```

С другой ветки:

```bash
curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | BRUHANET_REF=<branch> bash
```

Только конфиги и `.env`, без `pull` и `up`:

```bash
curl -fsSL https://raw.githubusercontent.com/bruhabruh/bruhanet-bot/main/install.sh | BRUHANET_SKIP_UP=1 bash
```

Повторный запуск обновляет конфиги и перезапускает стек, существующий `.env` не трогает.
