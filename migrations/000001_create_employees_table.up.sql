-- migrations/000001_create_employees_table.up.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS employees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_id BIGINT DEFAULT 0,
    token TEXT,
    first_name TEXT,
    patronymic TEXT,
    last_name TEXT,
    email TEXT,
    birth_date DATE NOT NULL,
    temp_password TEXT,
    subscribe JSONB NOT NULL DEFAULT '{}',
    wait_login BOOLEAN DEFAULT FALSE,
    wait_subscribe BOOLEAN DEFAULT FALSE,
    wait_unsubscribe BOOLEAN DEFAULT FALSE,
    in_tg_group BOOLEAN DEFAULT FALSE
);

-- функция импорта в базу из файла
CREATE OR REPLACE PROCEDURE import_from_csv(
    IN table_name TEXT DEFAULT 'employees',
    IN file_path TEXT DEFAULT '/var/lib/postgresql/csv/employees.csv',
    separator CHAR(1) DEFAULT ','
)
LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE 'COPY ' || table_name || ' FROM ''' || file_path || ''' WITH CSV DELIMITER ''' || separator || ''';';
END;
$$;

-- функция экспорта из базы в файл
CREATE OR REPLACE PROCEDURE export_to_csv(
    IN table_name TEXT DEFAULT 'employees',
    IN file_path TEXT DEFAULT '/var/lib/postgresql/csv/employees_export.csv',
    separator CHAR(1) DEFAULT ','
)
LANGUAGE plpgsql AS $$
BEGIN
    EXECUTE 'COPY ' || table_name || ' TO ''' || file_path || ''' WITH CSV DELIMITER ''' || separator || ''';';
END;
$$;

-- вызов импорта
CALL import_from_csv();
