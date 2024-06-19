package handle

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	td "birthdayGreetings/internal/TDlib"
	"birthdayGreetings/internal/db"
	m "birthdayGreetings/internal/mailer"

	tb "gopkg.in/telebot.v3"

	"github.com/gofrs/uuid"
)

type Handle struct {
	db db.DB
}

func NewHandle() *Handle {
	return &Handle{
		db: db.NewDB(),
	}
}

func (h *Handle) CloseDB() {
	h.db.Close()
}

// Login аутентификация
func (h *Handle) Login(c tb.Context) error {
	return c.Send("Пожалуйста, введите ваш email:")
}

// isValidEmail проверка валидности email
func isValidEmail(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	return re.MatchString(email)
}

// WaitUserResponse ожидание ответов пользователей
func (h *Handle) WaitUserResponse(c tb.Context) error {
	var err error
	var employee db.Employee
	var isValidEmailVal bool
	response := c.Message().Text

	// Если отправили валидный email
	if isValidEmailVal = isValidEmail(response); !isValidEmailVal {
		// Получаем данные из db по TelegramId
		if employee, err = h.db.GetEmployee(db.Employee{TelegramID: c.Sender().ID}); err != nil {
			log.Println(err)
			return h.BotHelp(c)
		}
	}

	if isValidEmailVal {
		return h.waitEmail(c, response)
	} else if employee.WaitLogin {
		return h.waitPassword(employee, c, response)
	} else if employee.WaitSubscribe {
		return h.waitSubscribe(employee, c, response)
	} else if employee.WaitUnsubscribe {
		return h.waitUnSubscribe(employee, c, response)
	}

	return h.BotHelp(c)
}

// waitEmail получает email от пользователя и отправляет на него пароль
func (h *Handle) waitEmail(c tb.Context, response string) error {
	// Проверяем email в базе
	employee, err := h.db.AuthenticateUser(c, response)
	if err != nil {
		return c.Send("Ошибка при аутентификации: " + err.Error())
	}

	// Отправляем временный пароль на email пользователя
	password, err := m.SendPasswordToEmail(employee.Email)
	if err != nil {
		return c.Send("Ошибка при отправке пароля: " + err.Error())
	}

	// Сохраняем временный пароль для дальнейшей проверки
	if err := h.db.PatchEmployee(db.Employee{ID: employee.ID, TempPassword: password, WaitLogin: true}, "TempPassword"); err != nil {
		return c.Send("Ошибка при аутентификации, попробуйте еще раз")
	}

	return c.Send("Мы отправили временный пароль на ваш email. Пожалуйста, введите его.")
}

// waitPassword получает пароль от пользователя и сравнивает с отправленным
func (h *Handle) waitPassword(employee db.Employee, c tb.Context, response string) error {
	// Проверяем введенный пароль
	if employee.TempPassword != response {
		// Ставим флаг employee.WaitLogin в false чтобы была одна попытка проверки пароля
		if err := h.db.PatchEmployee(db.Employee{ID: employee.ID}, "Wait"); err != nil {
			return err
		}
		return c.Send("Неверный пароль")
	} else {
		// Генерируем JWT-токен для пользователя
		token, err := db.GenerateJWTToken(employee.ID)
		if err != nil {
			return c.Send("Ошибка при генерации токена: " + err.Error())
		}
		// Сохраняем JWT-токен в базу
		if err = h.db.PatchEmployee(db.Employee{ID: employee.ID, Token: token}, "Token"); err != nil {
			return c.Send("Ошибка при аутентификации, попробуйте еще раз")
		}

		// Отправляем сообщение с успешной аутентификацией
		return c.Send("Аутентификация прошла успешно!")
	}
}

// waitSubscribe получает uuid сотрудников на которых нужно подписаться
func (h *Handle) waitSubscribe(employee db.Employee, c tb.Context, response string) error {
	data := strings.Fields(response)
	if len(data) == 0 {
		return h.SubscribeToNotifications(c)
	}

	hours, id, subscribe := 0, uuid.Nil, db.Employee{}
	for _, s := range data {
		// если это uuid
		if uuid, err := uuid.FromString(s); err == nil {
			id = uuid
			hours = 0
			// проверит на существование в db
			if subscribe, err = h.db.GetEmployee(db.Employee{ID: id}); err != nil {
				return c.Send(fmt.Sprintf("Вы отправили некорректные данные %s", s))
			}
		} else {
			// если не uuid - пробуем считать время до оповещания
			if hours, err = strconv.Atoi(s); err != nil {
				return c.Send(fmt.Sprintf("Вы отправили некорректные данные %s", s))
			}
		}

		if id != uuid.Nil {
			duration := time.Duration(hours) * time.Hour
			dateNotification := subscribe.BirthDate.Add(-duration)

			// Создаем новое время с текущим годом и старыми остальными полями
			dateNotification = time.Date(time.Now().Year(), dateNotification.Month(), dateNotification.Day(), dateNotification.Hour(),
				dateNotification.Minute(), dateNotification.Second(), dateNotification.Nanosecond(), dateNotification.Location())

			if dateNotification.Truncate(time.Hour).Before(time.Now().Truncate(time.Hour)) {
				// Создаем новое время в следующем году
				dateNotification = time.Date(time.Now().Year()+1, dateNotification.Month(), dateNotification.Day(), dateNotification.Hour(),
					dateNotification.Minute(), dateNotification.Second(), dateNotification.Nanosecond(), dateNotification.Location())
			}

			employee.Subscribe[id] = dateNotification
		}
	}

	// патчим в db
	err := h.db.PatchEmployee(db.Employee{ID: employee.ID, Subscribe: employee.Subscribe}, "Subscribe")
	if err != nil {
		log.Println(err)
		return c.Send("Ошибка, попробуйте еще раз")
	}

	return c.Send("Вы успешно подписались на оповещания!\n/subscribed - проверить список (на кого подписан)")
}

// waitUnSubscribe получает uuid сотрудников от которых нужно отписаться
func (h *Handle) waitUnSubscribe(employee db.Employee, c tb.Context, response string) error {
	data := strings.Fields(response)
	if len(data) == 0 {
		// на исходную позицию UnSubscribe
		return h.UnsubscribeFromNotifications(c)
	}

	for _, i := range data {
		uuid, err := uuid.FromString(i)
		if err != nil {
			return c.Send(fmt.Sprintf("Вы отправили некорректные данные %s", i))
		}
		// если uuid не существует - отправит предупреждение
		if _, ok := employee.Subscribe[uuid]; !ok {
			c.Send(fmt.Sprintf("%s - в вашем списке нет", i))
		}

		// удаляет из map
		delete(employee.Subscribe, uuid)
	}

	// патчит в bd
	err := h.db.PatchEmployee(db.Employee{ID: employee.ID, Subscribe: employee.Subscribe}, "Subscribe")
	if err != nil {
		log.Println(err)
		return c.Send("Ошибка, попробуйте еще раз")
	}

	return c.Send("Вы успешно отписались от оповещаний.\n/subscribed - проверить список (на кого подписан)")
}

// SubscribeToNotifications функция для подписки на уведомления о днях рождения
func (h *Handle) SubscribeToNotifications(c tb.Context) error {
	employee, err := h.authMiddleware(c)
	if err != nil {
		c.Send("Пожалуйста, пройдите аутентификацию:\n/login")
		return err
	}

	message := "Отправьте UUID сотрудников (да, этот длинный ключ), " +
		"на которых хотите подписаться.\nЧерез пробел можно указать " +
		"за сколько часов оповещать до дня рождения (одним числом), " +
		"или оповещание прийдет в наступивший День рождения сотрудника." +
		"\nНапример:\n\nb559d2f8-7319-4abb-8d8e-df7c98acff57 15\n" +
		"9abbc7d6-16b6-4376-b5f2-4d9633e940f1\n" +
		"c14edf46-2df1-4e1b-9d01-60d4f4dd3b99 20\n\n" +
		"/list - получить список сотрудников"
	err = c.Send(message)
	if err != nil {
		log.Println(err)
		return c.Send("Ошибка, попробуйте еще раз")
	}
	// Ставим флаг ожидания uuid сотрудников true
	return h.db.PatchEmployee(db.Employee{ID: employee.ID, WaitSubscribe: true}, "Wait")
}

// UnsubscribeFromNotifications функция для отписки от уведомлений о днях рождения
func (h *Handle) UnsubscribeFromNotifications(c tb.Context) error {
	employee, err := h.authMiddleware(c)

	if err != nil {
		c.Send("Пожалуйста, пройдите аутентификацию:\n/login")
		return err
	}

	message := "Отправьте UUID сотрудников " +
		"(да, снова этот длинный ключ), от которых хотите отписаться.\n" +
		"Например:\n\nb559d2f8-7319-4abb-8d8e-df7c98acff57\n" +
		"9abbc7d6-16b6-4376-b5f2-4d9633e940f1\n" +
		"c14edf46-2df1-4e1b-9d01-60d4f4dd3b99\n\n" +
		"/list - получить список сотрудников"
	err = c.Send(message)
	if err != nil {
		log.Println(err)
		return c.Send("Ошибка, попробуйте еще раз")
	}

	return h.db.PatchEmployee(db.Employee{ID: employee.ID, WaitUnsubscribe: true}, "Wait")
}

// Subscribed отправляет пользователю csv со списком (на кого он подписан)
func (h *Handle) Subscribed(c tb.Context) error {
	_, err := h.authMiddleware(c)

	if err != nil {
		c.Send("Пожалуйста, пройдите аутентификацию:\n/login")
		return err
	}

	file, err := os.Create("subscribed.csv")
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer file.Close()
	defer os.Remove(file.Name())

	// Записываем заголовок в файл
	_, err = file.WriteString("UUID,First_name,Patronymic,Last_name, Birth_date, Notification\n")
	if err != nil {
		fmt.Println(err)
		return err
	}

	e, err := h.db.GetEmployee(db.Employee{TelegramID: c.Sender().ID})
	if err != nil {
		log.Println(err)
		return c.Send("Ошибка, попробуйте еще раз")
	}

	for i, j := range e.Subscribe {
		employee, err := h.db.GetEmployee(db.Employee{ID: i})
		if err != nil {
			log.Println(err)
			return c.Send("Ошибка, попробуйте еще раз")
		}

		_, err = file.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s\n",
			employee.ID, employee.FirstName, employee.Patronymic, employee.LastName, employee.BirthDate, j))
		if err != nil {
			fmt.Println(err)
			return err
		}
	}
	// Отправляем файл боту
	doc := &tb.Document{
		File:     tb.FromDisk(file.Name()),
		FileName: filepath.Base(file.Name()),
		MIME:     "text/csv",
	}
	c.Send(doc)

	return nil
}

// List отправляет пользователю csv со списком сотрудников
func (h *Handle) List(c tb.Context) error {
	_, err := h.authMiddleware(c)

	if err != nil {
		c.Send("Пожалуйста, пройдите аутентификацию:\n/login")
		return err
	}
	// Создаем файл .CSV
	file, err := os.Create("employees.csv")
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer file.Close()
	defer os.Remove(file.Name())

	// Записываем заголовок в файл
	_, err = file.WriteString("UUID,First_name,Patronymic,Last_name, Birth_date\n")
	if err != nil {
		fmt.Println(err)
		return err
	}

	page := 0
	count, err := h.db.GetCount()
	if err != nil {
		fmt.Println(err)
		return err
	}

	for {
		employees, err := h.db.GetPage(page)
		if err != nil {
			fmt.Println(err)
			return err
		}
		// Записываем данные в файл
		for _, employee := range employees {
			_, err = file.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s\n",
				employee.ID, employee.FirstName, employee.Patronymic, employee.LastName, employee.BirthDate))
			if err != nil {
				fmt.Println(err)
				return err
			}
		}
		// Проверяем, есть ли еще страницы
		if page*db.LIMIT >= count {
			break
		}
		// Переходим к следующей странице
		page++
	}
	// Отправляем файл боту
	doc := &tb.Document{
		File:     tb.FromDisk(file.Name()),
		FileName: filepath.Base(file.Name()),
		MIME:     "text/csv",
	}
	c.Send(doc)

	return nil
}

// Scheduler
func (h *Handle) Scheduler(b *tb.Bot) {
	var t td.TDlib

	groupID, err := strconv.ParseInt(os.Getenv("TELEGRAM_GROUP"), 10, 64)
	if err != nil {
		log.Fatal("Невалидный TELEGRAM_GROUP")
		return
	}

	apiID, err := strconv.Atoi(os.Getenv("API_ID"))
	if err != nil {
		panic("Невалидный API_ID")
	}

	t, err = td.NewTDlib(int32(apiID), os.Getenv("API_HASH"))
	if err != nil {
		panic(err.Error())
	}
	defer t.TDlibStop()

	// Проверяем, существует ли группа
	_, err = t.GetGroup(groupID)
	if err != nil {
		// Создаем новую группу
		groupID, err = t.CreateNewGroup(os.Getenv("NAME_TELEGRAM_GROUP"))
		if err != nil {
			panic(err)
		}

		_, err = t.GetGroup(groupID)
		if err != nil {
			panic(err)
		}

		// заменит groupID
		changeEnv(groupID)
	}

	if err = h.SchedulerNotifications(t, b, groupID); err != nil {
		log.Println(err)
	}

	for {
		// Получаем текущее время
		now := time.Now()
		// Определяем время следующего часа в 00 минут
		nextHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
		// Вычисляем, сколько времени осталось до следующего часа в 00 минут
		waitTime := nextHour.Sub(now)
		// Ждём до следующего часа в 00 минут
		time.Sleep(waitTime)
		fmt.Println("Scheduler выполняет проверку: ", time.Now().Format("2006-01-02 15:04:05"))

		if err = h.SchedulerNotifications(t, b, groupID); err != nil {
			log.Println(err)
		}
	}
}

// changeEnv заменит "TELEGRAM_GROUP=" в .env на актуальные данные
func changeEnv(groupID int64) {
	groupIDstr := strconv.Itoa(int(groupID))

	oldID := os.Getenv("TELEGRAM_GROUP")

	// Путь к файлу .env
	envFilePath := "../configs/.env"

	// Строка для поиска
	searchString := fmt.Sprintf(`TELEGRAM_GROUP=%s`, oldID)

	// Новая строка для замены
	replaceString := fmt.Sprintf(`TELEGRAM_GROUP=%s`, groupIDstr)

	// Открытие файла для чтения
	file, err := os.Open(envFilePath)
	if err != nil {
		fmt.Println("Ошибка открытия .env:", err)
		return
	}
	defer file.Close()

	// Создание нового файла для записи
	newFile, err := os.Create(envFilePath + ".tmp")
	if err != nil {
		fmt.Println("Ошибка создания нового файла .env:", err)
		return
	}
	defer newFile.Close()

	// Чтение содержимого файла
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Проверка, содержит ли строка искомое значение
		if strings.Contains(line, searchString) {
			// Замена искомой строки на новую
			line = strings.ReplaceAll(line, searchString, replaceString)
		}
		// Запись обновленной строки в новый файл
		_, err = newFile.WriteString(line + "\n")
		if err != nil {
			fmt.Println("Ошибка записи в новый файл .env:", err)
			return
		}
	}

	// Переименование временного файла в оригинальный
	err = os.Rename(envFilePath+".tmp", envFilePath)
	if err != nil {
		fmt.Println("Ошибка переименования файла .env:", err)
		return
	}
}

// SchedulerNotifications функция по сегментам достает данные из db для проверки
func (h *Handle) SchedulerNotifications(t td.TDlib, b *tb.Bot, groupID int64) error {
	page := 0
	count, err := h.db.GetCount()
	if err != nil {
		fmt.Println(err)
		return err
	}

	for {
		employees, err := h.db.GetPage(page)
		if err != nil {
			fmt.Println(err)
			return err
		}

		if err = h.checkEmployees(employees, t, b, groupID); err != nil {
			return err
		}

		// Проверяем, есть ли еще страницы
		if page*db.LIMIT >= count {
			break
		}
		// Переходим к следующей странице
		page++
	}

	return nil
}

// checkEmployees проверяет каждого пользователя
func (h *Handle) checkEmployees(employees []db.Employee, t td.TDlib, b *tb.Bot, groupID int64) error {
	for _, employee := range employees {
		if len(employee.Subscribe) != 0 {
			// Если пользователь подписан на кого-то и не состоит в группе
			if !employee.InTgGroup {
				// Добавляем его в группу
				err := t.AddUserToGroup(groupID, employee.TelegramID)
				if err != nil {
					log.Println(err)
				} else {
					h.db.PatchEmployee(db.Employee{ID: employee.ID, InTgGroup: true}, "InTgGroup")
				}
			}

			// Проверяем, если у него День рождения -
			if employee.BirthDate.Month() == time.Now().Month() &&
				employee.BirthDate.Day() == time.Now().Day() && employee.BirthDate.Hour() == time.Now().Hour() {
				// бот отправит поздравление
				message := "Поздравляю тебя с Днём рождения! Желаю тебе исполнения всех твоих " +
					"мечтаний и достижения поставленных целей. Пусть успех сопровождает " +
					"тебя всегда и во всём, а здоровье будет крепким, как алмаз!"

				b.Send(&tb.Chat{ID: employee.TelegramID}, message)
			}

			newSubscribe, flag := make(map[uuid.UUID]time.Time), false
			// Проверяем время оповещания его подписок
			for k, t := range employee.Subscribe {
				newSubscribe[k] = t

				if t.Month() == time.Now().Month() && t.Day() == time.Now().Day() && t.Hour() == time.Now().Hour() {
					// Если пришло время - оповещаем о Дне рождения у сотрудника, на которого подписан
					e, err := h.db.GetEmployee(db.Employee{ID: k})
					if err != nil {
						return err
					}

					message := fmt.Sprintf("Самое время напомнить!\n\n %s %s %s - День рождения %v.\n\n Не забудьте поздравить!",
						e.FirstName, e.Patronymic, e.LastName, e.BirthDate)

					_, err = b.Send(&tb.Chat{ID: employee.TelegramID}, message)
					if err != nil {
						log.Println("Ошибка оповещания о Дне рожденния:", err)
					}
					// Обновляем дату оповещания
					newDateNotification := time.Date(time.Now().Year()+1, t.Month(), t.Day(),
						t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())

					newSubscribe[k] = newDateNotification
					flag = true
				}
			}
			if flag {
				err := h.db.PatchEmployee(db.Employee{ID: employee.ID, Subscribe: newSubscribe}, "Subscribe")
				if err != nil {
					log.Println("Ошибка сохранения новой даты оповещания:", err)
				}
			}
		}
	}

	return nil
}

// authMiddleware проверка авторизации у пользователей
func (h *Handle) authMiddleware(c tb.Context) (db.Employee, error) {
	// Получает JWT-токен из контекста сообщения
	employee, err := h.db.GetEmployee(db.Employee{TelegramID: c.Sender().ID})
	if err != nil {
		return db.Employee{}, errors.New("ошибка получения токена: " + err.Error())
	}

	jwtToken := employee.Token
	if jwtToken == "" {
		// Если токен недоступен - отклонит сообщение
		return db.Employee{}, errors.New("токен не доступен")
	}

	// Верифицирует и распарсит токен
	if err = db.JwtParse(jwtToken); err != nil {
		return db.Employee{}, err
	}

	return employee, nil
}

// BotStart обработка команды /start
func (h *Handle) BotStart(c tb.Context) error {
	if err := c.Send("Привет! Я, бот-помощник, для поздравлений сотрудников с Днем рождения)"); err != nil {
		return err
	}

	return h.BotHelp(c)
}

// BotHelp команда /help
func (h *Handle) BotHelp(c tb.Context) error {
	if err := c.Send("/login - пройти аутентификацию"); err != nil {
		return err
	}
	if err := c.Send("/subscribe - подписаться на оповещения о Дне рождения"); err != nil {
		return err
	}
	if err := c.Send("/unsubscribe - отписаться от оповещений о Дне рождения"); err != nil {
		return err
	}
	if err := c.Send("/list - получить список сотрудников"); err != nil {
		return err
	}
	if err := c.Send("/subscribed - проверить список (на кого подписан)"); err != nil {
		return err
	}

	return nil
}
