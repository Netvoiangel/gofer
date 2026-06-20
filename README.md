# Telegram-бот «Гофер»

«Гофер» — Telegram-бот для группового чата: отвечает на прямые обращения, иногда реагирует на технические темы, умеет писать редкие инициативные сообщения и держит образ злого, недовольного, матерящегося Go-персонажа.

## Что уже есть

- Long polling через Telegram Bot API.
- Polza.AI через OpenAI-совместимый `chat/completions`.
- Команды `/gopher_help`, `/gopher_on`, `/gopher_off`, `/gopher_silent`, `/gopher_mode`, `/gopher_stats`, `/gopher_budget`, `/gopher_reset_context`.
- Антиспам-лимиты: cooldown, максимум ответов в час, дневной бюджет токенов, максимум инициативных сообщений.
- Краткосрочный контекст последних сообщений и простое резюме тем.
- Настройки чата, статистика и события в локальном JSON-хранилище.
- Podman Compose запуск.

## Важные условия Telegram

Чтобы бот видел обычные сообщения в группе, ему нужны права администратора или отключённый privacy mode в BotFather. Иначе Telegram будет передавать только команды, упоминания и часть служебных событий.

## Быстрый запуск

1. Создайте `.env` на основе `.env.example`.
2. Заполните `TELEGRAM_BOT_TOKEN`, `POLZA_API_KEY`, `POLZA_MODEL`.
3. Если хотите дать права владельцам без проверки Telegram-админки, укажите `ADMIN_USER_IDS` через запятую.
4. Запустите:

```text
podman compose up -d --build
```

Локальный запуск без Docker:

```text
go run ./cmd/bot
```

## Основные переменные

```env
TELEGRAM_BOT_TOKEN=
POLZA_API_KEY=
POLZA_BASE_URL=https://api.polza.ai/api/v1
POLZA_MODEL=
STORAGE_PATH=data/state.json
LOG_LEVEL=info
```

Поведение и лимиты настраиваются через переменные `BOT_*` и `PROB_*` из `.env.example`.

## Команды

- `/gopher_help` — список команд.
- `/gopher_on` — включить активность.
- `/gopher_off` — выключить активность.
- `/gopher_silent 60` — отключить инициативные реакции на 60 минут.
- `/gopher_mode calm` — спокойный режим.
- `/gopher_mode funny` — более юмористический режим.
- `/gopher_mode tech` — технический режим.
- `/gopher_mode angry` — злой ворчливый режим по умолчанию.
- `/gopher_stats` — статистика сообщений, токенов и срабатываний.
- `/gopher_budget` — текущие лимиты.
- `/gopher_reset_context` — очистить краткосрочный контекст.

Команды изменения режима и активности доступны администраторам чата или пользователям из `ADMIN_USER_IDS`.

## Хранилище

Для MVP используется JSON-файл из `STORAGE_PATH`; по умолчанию это `data/state.json`. В контейнере файл лежит внутри `/app/data`, а директория `./data` подключена volume в `compose.yml`, поэтому состояние сохраняется между перезапусками. Хранилище содержит настройки чатов, последние сообщения, резюме контекста, события и статистику. Тексты сообщений можно не сохранять, если установить:

```env
BOT_STORE_TEXT=false
```

## Деплой через Podman Compose

### Подготовка сервера

```bash
sudo mkdir -p /opt/gofer
sudo chown -R $USER:$USER /opt/gofer
cd /opt/gofer
```

Проверьте, какой compose-инструмент установлен:

```bash
which podman
which podman-compose
```

Основной вариант в этом проекте — `podman compose`. Если на сервере установлен только `podman-compose`, используйте те же команды, заменив `podman compose` на `podman-compose`.

### Загрузка проекта

Через git:

```bash
git clone <repo-url> .
```

Или загрузите архив/scp в `/opt/gofer` и распакуйте файлы проекта туда.

### Настройка окружения

```bash
cp .env.example .env
nano .env
chmod 600 .env
mkdir -p data
```

Минимально заполните:

```env
TELEGRAM_BOT_TOKEN=
POLZA_API_KEY=
POLZA_BASE_URL=https://api.polza.ai/api/v1
POLZA_MODEL=
STORAGE_PATH=data/state.json
LOG_LEVEL=info
BOT_MIN_DELAY_SECONDS=180
BOT_MAX_REPLIES_PER_HOUR=10
BOT_MAX_PROACTIVE_PER_DAY=5
POLZA_SILENT_ON_MISSING=true
```

### Запуск

```bash
podman compose build
podman compose up -d
podman compose logs -f
```

### Проверка

```bash
podman ps
podman logs -f gofer-bot
```

Проверьте, что файл состояния появился в `data/state.json`:

```bash
ls -la data
```

### Остановка

```bash
podman compose down
```

### Обновление

```bash
cd /opt/gofer
git pull
podman compose up -d --build
podman logs --tail=100 gofer-bot
```

### Автозапуск через systemd user service

```bash
mkdir -p ~/.config/systemd/user
cp deploy/systemd/gofer.service ~/.config/systemd/user/gofer.service
systemctl --user daemon-reload
systemctl --user enable --now gofer.service
sudo loginctl enable-linger $USER
systemctl --user status gofer.service
```

Если используется `podman-compose`, замените в `~/.config/systemd/user/gofer.service` строки запуска:

```ini
ExecStart=/usr/bin/podman-compose up -d
ExecStop=/usr/bin/podman-compose down
```

## Дальнейшие улучшения

- Заменить JSON-хранилище на SQLite или PostgreSQL.
- Добавить полноценные миграции.
- Добавить webhook-режим для Telegram.
- Расширить долгосрочную память и allowlist тем.
- Добавить метрики и healthcheck.

## Техническая приёмка MVP

Перед передачей проекта обязательно выполнить:

```bash
go version
go mod tidy
gofmt -w .
go test ./...
go vet ./...
podman compose build
```

Проверка запуска:

```bash
podman compose up -d
podman compose logs -f
```

Ручная проверка в Telegram-группе:

- контейнер стартует без panic;
- бот подключается к Telegram;
- LLM-запрос к Polza.AI успешно проходит;
- `/gopher_help` показывает команды;
- `/gopher_on` и `/gopher_off` меняют состояние;
- прямое упоминание `@username` получает ответ при доступных лимитах;
- reply на сообщение бота получает ответ при доступных лимитах;
- обычное сообщение иногда получает реакцию по правилам вероятности;
- cooldown и часовой лимит не дают боту отвечать слишком часто;
- в логах видна причина ответа или пропуска;
- после перезапуска контейнера настройки чата сохраняются.

Проект не считается принятым, пока не пройдены `go test ./...`, `go vet ./...` и запуск через Podman Compose.
