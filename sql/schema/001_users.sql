-- +goose Up
CREATE TABLE users(
	id UUID PRIMARY KEY,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	email TEXT UNIQUE NOT NULL,
	hashed_password TEXT NOT NULL DEFAULT 'unset',
	is_chirpy_red BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE chirps(
	id UUID PRIMARY KEY,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	body TEXT NOT NULL,
	user_id UUID NOT NULL,

	CONSTRAINT fk_user
		FOREIGN KEY (user_id)
		REFERENCES users(id)
		ON DELETE CASCADE
);
CREATE TABLE refresh_tokens(
	token TEXT PRIMARY KEY,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	user_id UUID NOT NULL,
	expires_at TIMESTAMP NOT NULL,
	revoked_at TIMESTAMP,

	CONSTRAINT fk_user_token
		FOREIGN KEY (user_id)
		REFERENCES users(id)
		ON DELETE CASCADE
);
-- +goose Down
DROP TABLE refresh_tokens;
DROP TABLE chirps;
DROP TABLE users;
