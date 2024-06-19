package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gofrs/uuid"
	"github.com/golang-migrate/migrate"
	"github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/source/file"
	tb "gopkg.in/telebot.v3"
)

const LIMIT = 10

type Employee struct {
	ID              uuid.UUID               `json:"id"`
	TelegramID      int64                   `json:"telegram_id"`
	Token           string                  `json:"token"`
	FirstName       string                  `json:"first_name"`
	Patronymic      string                  `json:"patronymic"`
	LastName        string                  `json:"last_name"`
	Email           string                  `json:"email"`
	BirthDate       time.Time               `json:"birth_date"`
	TempPassword    string                  `json:"temp_password"`
	Subscribe       map[uuid.UUID]time.Time `json:"subscribe"`
	WaitLogin       bool                    `json:"wait_login"`
	WaitSubscribe   bool                    `json:"wait_subscribe"`
	WaitUnsubscribe bool                    `json:"wait_unsubscribe"`
	InTgGroup       bool                    `json:"in_tg_group"`
}

type DB struct {
	dB *sql.DB
}

func NewDB() DB {
	dbPort, err := strconv.Atoi(os.Getenv("DB_PORT"))
	if err != nil {
		log.Println("Введите номер порта Postgres:")
		fmt.Scan(&dbPort)
	}

	dbURL := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"), dbPort, os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_DB"))

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Ошибка соединения с Postgres: ", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatal(err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s/../migrations", os.Getenv("PWD")),
		"postgres", driver)
	if err != nil {
		log.Fatal("Ошибка migrate: ", err)
	}

	row := db.QueryRow("SELECT COUNT( * ) FROM employees")
	var count int
	if err := row.Scan(&count); err != nil {
		if err := m.Up(); err != nil {
			log.Fatal(err)
		}
	}

	return DB{dB: db}
}

func (d *DB) Close() {
	d.dB.Close()
}

// AuthenticateUser проверит авторизацию пользователя
func (d *DB) AuthenticateUser(c tb.Context, email string) (Employee, error) {
	employee, err := d.GetEmployee(Employee{Email: email})
	if err != nil {
		return Employee{}, errors.New("email не найден")
	}

	if err = JwtParse(employee.Token); err != nil {
		employee.TelegramID = c.Sender().ID
		if err := d.PatchEmployee(Employee{ID: employee.ID, TelegramID: employee.TelegramID}, "TelegramID"); err != nil {
			log.Println(err)
			return Employee{}, errors.New("ошибка аутентификации. Пожалуйста, повторите")
		}
	}

	if c.Sender().ID == employee.TelegramID {
		return employee, nil
	}

	return Employee{}, errors.New("telegram ID не от вашей учетной записи")
}

// PatchEmployee сохраняет изменения в db
func (d *DB) PatchEmployee(e Employee, field string) error {
	if field == "TelegramID" {
		_, err := d.dB.Exec(
			`UPDATE employees e SET telegram_id = $1
		     WHERE e.id = $2`, e.TelegramID, e.ID)
		if err != nil {
			return err
		}
	}
	if field == "Token" {
		_, err := d.dB.Exec(
			`UPDATE employees e
			 SET token = $1, wait_login = $2, wait_subscribe = $3, wait_unsubscribe = $4
		     WHERE e.id = $5`, e.Token, e.WaitLogin, e.WaitSubscribe, e.WaitUnsubscribe, e.ID)
		if err != nil {
			return err
		}
	}
	if field == "Wait" {
		_, err := d.dB.Exec(
			`UPDATE employees e
			 SET wait_login = $1, wait_subscribe = $2, wait_unsubscribe = $3
		     WHERE e.id = $4`, e.WaitLogin, e.WaitSubscribe, e.WaitUnsubscribe, e.ID)
		if err != nil {
			return err
		}
	}
	if field == "InTgGroup" {
		_, err := d.dB.Exec(
			`UPDATE employees e SET in_tg_group = $1
		     WHERE e.id = $2`, e.InTgGroup, e.ID)
		if err != nil {
			return err
		}
	}
	if field == "TempPassword" {
		_, err := d.dB.Exec(
			`UPDATE employees e
			 SET temp_password = $1, wait_login = $2, wait_subscribe = $3, wait_unsubscribe = $4
		     WHERE e.id = $5`, e.TempPassword, e.WaitLogin, e.WaitSubscribe, e.WaitUnsubscribe, e.ID)
		if err != nil {
			return err
		}
	}
	if field == "Subscribe" {
		var err error
		var subscribeBytes []byte
		if subscribeBytes, err = json.Marshal(&e.Subscribe); err != nil {
			return err
		}
		_, err = d.dB.Exec(
			`UPDATE employees e
			 SET subscribe = $1, wait_login = $2, wait_subscribe = $3, wait_unsubscribe = $4
		     WHERE e.id = $5`, subscribeBytes, e.WaitLogin, e.WaitSubscribe, e.WaitUnsubscribe, e.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetPage извлекает данные сегментами (страницами)
func (d *DB) GetPage(page int) ([]Employee, error) {
	offset := page * LIMIT

	rows, err := d.dB.Query(
		`SELECT * 
		FROM employees e
		LIMIT $1 OFFSET $2`,
		LIMIT, offset)

	var employees []Employee
	if err == nil {
		for rows.Next() {
			var e Employee
			var subscribeBytes []byte

			err = rows.Scan(
				&e.ID, &e.TelegramID, &e.Token, &e.FirstName, &e.Patronymic, &e.LastName, &e.Email, &e.BirthDate,
				&e.TempPassword, &subscribeBytes, &e.WaitLogin, &e.WaitSubscribe, &e.WaitUnsubscribe, &e.InTgGroup)

			if err != nil {
				return []Employee{}, err
			}

			if len(subscribeBytes) > 0 {
				if err := json.Unmarshal(subscribeBytes, &e.Subscribe); err != nil {
					return []Employee{}, err
				}
			}
			employees = append(employees, e)
		}
		if err = rows.Err(); err != nil {
			return []Employee{}, err
		}
	} else {
		return []Employee{}, err
	}

	return employees, nil
}

// GetCount возвращает количество пользователей (строк)
func (d *DB) GetCount() (int, error) {
	var count int
	err := d.dB.QueryRow("SELECT COUNT( * ) FROM employees").Scan(&count)

	return count, err
}

// GetEmployee извлекает данные одного пользователя
func (d *DB) GetEmployee(e Employee) (Employee, error) {
	var err error
	var rows *sql.Rows

	switch {
	case e.Email != "":
		rows, err = d.dB.Query(
			`SELECT *
			FROM employees e
			WHERE e.email = $1`,
			e.Email)
	case e.TelegramID != 0:
		rows, err = d.dB.Query(
			`SELECT *
			FROM employees e
			WHERE e.telegram_id = $1`,
			e.TelegramID)
	case e.ID != uuid.Nil:
		rows, err = d.dB.Query(
			`SELECT *
				FROM employees e
				WHERE e.id = $1`,
			e.ID)
	}

	if err == nil {
		rows.Next()
		var subscribeBytes []byte

		err = rows.Scan(
			&e.ID, &e.TelegramID, &e.Token, &e.FirstName, &e.Patronymic, &e.LastName, &e.Email, &e.BirthDate,
			&e.TempPassword, &subscribeBytes, &e.WaitLogin, &e.WaitSubscribe, &e.WaitUnsubscribe, &e.InTgGroup)

		if len(subscribeBytes) > 0 {
			if err := json.Unmarshal(subscribeBytes, &e.Subscribe); err != nil {
				return e, err
			}
		}

	}

	return e, err
}

// GenerateJWTToken генерирует токен для авторизации
func GenerateJWTToken(id uuid.UUID) (string, error) {
	// Установка claims для JWT-токена
	claims := jwt.MapClaims{
		"employee_id": id,
		"exp":         time.Now().Add(time.Hour * 24).Unix(), // Токен истекает через 24 часа
	}

	// Создает и подписывает JWT-токен
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(os.Getenv("SECRET")))
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

// JwtParse проверит токен пользователя
func JwtParse(jwtToken string) error {
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		// Проверит подпись токена
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("неверный метод подписи токена: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("SECRET")), nil
	})
	if err != nil || !token.Valid {
		// Отклонит сообщение, если токен недействителен
		return errors.New("токен недействителен")
	}

	return nil
}
