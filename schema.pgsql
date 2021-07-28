-- in reverse order because of FOREIGN KEYs
DROP TABLE IF EXISTS team_location_history;
DROP TABLE IF EXISTS cipher_status;
DROP TABLE IF EXISTS team_status;
DROP TABLE IF EXISTS messages;

CREATE TABLE team_status (
	team		text		PRIMARY KEY,
	lat		float		NOT NULL,
	lon		float		NOT NULL,
	last_moved	timestamptz	DEFAULT NULL,
	cooldown_to	timestamptz	DEFAULT NULL
);


CREATE TABLE cipher_status (
	cipher		text		NOT NULL,
	team		text		NOT NULL,
	arrival		timestamptz	NOT NULL,
	solved		timestamptz	DEFAULT NULL,
	hint		timestamptz	DEFAULT NULL,
	skip		timestamptz	DEFAULT NULL,
	extra_points	int		DEFAULT 0,
	UNIQUE (cipher, team),
	FOREIGN KEY(team) REFERENCES team_status(team) ON DELETE CASCADE
);

CREATE TABLE team_location_history (
	team		text		NOT NULL,
	time		timestamptz	DEFAULT CURRENT_TIMESTAMP,
	lat		float		NOT NULL,
	lon		float		NOT NULL,
	FOREIGN KEY(team) REFERENCES team_status(team) ON DELETE CASCADE
);

CREATE TABLE messages (
	id		SERIAL		PRIMARY KEY,
	team		text		NOT NULL,
	cipher		text		NOT NULL,
	time		timestamptz	DEFAULT CURRENT_TIMESTAMP,
	phone_number	text		NOT NULL,
	sms_id		integer		NOT NULL,
	text		text		NOT NULL,
	response	text		NOT NULL
);

CREATE INDEX messages_sms_id ON messages(sms_id);
CREATE INDEX messages_team ON messages(team);
