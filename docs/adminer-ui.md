# Adminer (Admin Database UI)

[Adminer](https://www.adminer.org) (formerly phpMinAdmin) is a full-featured database management tool written in PHP. It serves as the admin UI for accessing and manipulating databases before we have time to implement a proper frontend.

## Setup

### Docker

Run the [official Docker image](https://hub.docker.com/_/adminer), which contains its own PHP runtime and requires no additional setup. By default, the instance is hosted at `http://localhost:8080`. You can use a different port by using Docker port mapping, e.g., `$ docker run -p 3000:8080 adminer`.

### PHP

Adminer is just a single PHP file which can be downloaded from the [official site](https://www.adminer.org/#download). It can then be run with a PHP runtime like `php` or `fastcgi`.

## Configuration

In the Adminer login UI, select `System` as `PostgreSQL`. Then enter the database credentials according to `config.toml`. For example, to access the folder preprocessor database, use the values under `config.FolderDatabase`.

## Usage

Select Database, table and data to view available data. The Adminer UI should be trivial to navigate and more information can be found [here](https://www.adminer.org/)
