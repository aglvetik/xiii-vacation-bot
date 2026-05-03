# XIII Vacation Bot

Discord-бот на Go для управления заявками на отпуск в клане XIII. Бот держит постоянную панель с кнопкой подачи заявки, отправляет заявки офицерам, выдаёт и снимает роль отпуска, поддерживает досрочное завершение через DM и автоматически завершает отпуска по сроку.

## Возможности

- Постоянная панель в канале заявок.
- Модальное окно Discord для ввода количества дней и причины.
- Проверка лимита дней через `MAX_VACATION_DAYS`.
- Запрет повторной активной заявки и повторного активного отпуска.
- Рассмотрение заявок офицерами через кнопки `Принять` и `Отклонить`.
- Проверка прав офицера по доступу к officer-каналу.
- Выдача роли отпуска при одобрении.
- DM пользователю с кнопкой досрочного завершения отпуска.
- Автоматический воркер завершения отпусков каждые 60 секунд.
- SQLite с миграциями и устойчивостью к `database is locked`.
- Готовый systemd unit для Linux VPS.

## Требования

- Go 1.22 или новее.
- Linux VPS для production-запуска.
- SQLite через `github.com/mattn/go-sqlite3`, поэтому нужен C-компилятор.

Для Ubuntu/Debian:

```bash
sudo apt update
sudo apt install -y build-essential git
```

## Права Discord-бота

Боту нужны права:

- View Channels
- Send Messages
- Embed Links
- Use External Emojis не обязателен
- Read Message History
- Manage Roles

Также в Developer Portal включите нужные Gateway Intents:

- Server Members Intent

Важно: роль бота на сервере должна быть выше роли отпуска `1498022112131289214`, иначе Discord не позволит выдавать и снимать эту роль.

## Настройка `.env`

Создайте файл `.env` из примера:

```bash
cp .env.example .env
nano .env
```

Пример:

```env
DISCORD_TOKEN=put_token_here
GUILD_ID=put_guild_id_here
PANEL_CHANNEL_ID=1500437958375903232
OFFICER_CHANNEL_ID=1500438001514184714
VACATION_ROLE_ID=1498022112131289214
DATABASE_PATH=./data/vacations.db
BRAND_NAME=XIII
MAX_VACATION_DAYS=30
LOG_LEVEL=info
```

Токен нельзя коммитить и нельзя публиковать в логах.

## Локальный запуск

```bash
go mod tidy
go run ./cmd/bot
```

При запуске бот:

1. загружает `.env`;
2. открывает SQLite;
3. запускает миграции;
4. обновляет существующую панель или создаёт новую;
5. сразу проверяет просроченные отпуска;
6. запускает воркер завершения каждые 60 секунд.

## Сборка

```bash
CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o xiii-vacation-bot ./cmd/bot
```

На Windows для локальной сборки `go-sqlite3` тоже требует рабочий C-компилятор. На VPS обычно проще собирать прямо на Linux.

## Деплой на VPS

Рекомендуемая директория совпадает с systemd unit:

```bash
sudo mkdir -p /opt/XIII
cd /opt/XIII
sudo git clone <your_repo_url> xiii-vacation-bot
cd xiii-vacation-bot
sudo cp .env.example .env
sudo nano .env
sudo ./scripts/install_systemd.sh
```

Если вы копируете проект без git, положите все файлы в:

```text
/opt/XIII/xiii-vacation-bot
```

## systemd

Проверить статус:

```bash
sudo systemctl status xiii-vacation-bot
```

Смотреть логи:

```bash
sudo journalctl -u xiii-vacation-bot -f
```

Перезапустить:

```bash
sudo systemctl restart xiii-vacation-bot
```

Остановить:

```bash
sudo systemctl stop xiii-vacation-bot
```

Отключить автозапуск:

```bash
sudo systemctl disable xiii-vacation-bot
```

## База данных

По умолчанию база лежит в:

```text
./data/vacations.db
```

Таблицы создаются автоматически:

- `bot_state`
- `vacation_requests`
- `vacations`

Панель хранится в `bot_state.panel_message_id`. Если сообщение панели удалили, бот создаст новое при следующем запуске.

## Troubleshooting

### Бот не выдаёт роль отпуска

Проверьте:

- у бота есть `Manage Roles`;
- роль бота выше роли отпуска;
- роль отпуска существует;
- `VACATION_ROLE_ID` указан правильно.

### Офицер не может принять или отклонить заявку

Бот проверяет не роли, а доступ к officer-каналу `1500438001514184714`. У пользователя должен быть `View Channel` для этого канала.

### Пользователь не получает DM

У пользователя могут быть закрыты личные сообщения. Это не ломает процесс: отпуск всё равно одобряется или отклоняется, а в officer-канал уходит предупреждение.

### `database is locked`

Бот включает SQLite busy timeout, WAL и повторяет операции при busy/locked ошибках. Если ошибка повторяется постоянно, проверьте, что одну и ту же базу не держит другой процесс.

### systemd сразу перезапускает сервис

Смотрите логи:

```bash
sudo journalctl -u xiii-vacation-bot -n 100 --no-pager
```

Частые причины:

- неверный `DISCORD_TOKEN`;
- не заполнен `GUILD_ID`;
- нет прав на директорию базы;
- бинарник не собран;
- unit указывает не на ту директорию.

### Ошибка сборки `go-sqlite3`

Установите C-компилятор:

```bash
sudo apt install -y build-essential
```

После этого повторите:

```bash
go mod tidy
CGO_ENABLED=1 go build -o xiii-vacation-bot ./cmd/bot
```
