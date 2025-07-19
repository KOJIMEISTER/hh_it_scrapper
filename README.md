# HH IT Scraper

![Go](https://img.shields.io/badge/Go-1.23.4-blue)
![MongoDB](https://img.shields.io/badge/MongoDB-latest-green)
![Docker](https://img.shields.io/badge/Docker-compose-orange)
![License](https://img.shields.io/badge/License-GPL--3.0-lightgrey)

A utility for collecting statistics on IT vacancies, companies, and technologies from HeadHunter API to analyze demand for tech skills in the Russian IT market.

## Features

- **Asynchronous API Scraping**: Efficiently fetches vacancy data from HeadHunter's public API
- **MongoDB Storage**: Stores collected data with proper indexing for fast queries
- **Duplicate Prevention**: Uses MD5 hashing to avoid duplicate vacancies
- **Concurrent Processing**: Processes multiple vacancies simultaneously (10 concurrent by default)
- **Daily Updates**: Bash script for scheduled daily updates
- **Comprehensive Logging**: Detailed logs for both successful operations and errors
- **Docker Support**: Ready-to-run containerized solution
- **Configurable Parameters**: Adjustable dates, areas, professional roles, and API settings

## Technology Stack

- **Language**: Golang 1.23.4
- **Database**: MongoDB (latest)
- **Containerization**: Docker with Docker Compose
- **Scripting**: Shell (Bash)

## Getting Started

### Prerequisites

- Docker and Docker Compose installed
- HeadHunter API bearer token
- MongoDB (can be run via Docker)

### Installation

1. Clone the repository:

```bash
git clone https://github.com/KOJIMEISTER/hh_it_scrapper.git
cd hh_it_scrapper
```

2. Create a `.env` file with your credentials:

```env
BEARER_TOKEN=your_hh_api_token
```

3. Build and start the containers:

```bash
docker-compose up --build
```

### Configuration

Configure the scraper by setting environment variables or command-line arguments:

| Parameter          | Description                              | Default                               |
| ------------------ | ---------------------------------------- | ------------------------------------- |
| `BEARER_TOKEN`     | HeadHunter API bearer token              | Required                              |
| `MONGO_URI`        | MongoDB connection string                | `mongodb://root:546258@mongodb:27017` |
| `--from`           | Start date in YYYY-MM-DD format          | Required                              |
| `--to`             | End date in YYYY-MM-DD format            | Required                              |
| `DOWNLOAD_30_DAYS` | Set to "true" to fetch last 30 days data | false                                 |

## Usage

### Running the Scraper

For one-time run:

```bash
./main --from 2024-01-01 --to 2024-01-02
```

For scheduled daily updates (via Docker):

```bash
docker-compose up
```

### Data Storage

Data is stored in MongoDB with the following structure:

- Database: `vacancy_db`
- Collection: `vacancies`
- Indexes:
  - `id` (unique)
  - `description_hash` (unique)

### Logging

Logs are available in the `logs/` directory:

- `info.log` - General operation logs
- `error.log` - Error messages and warnings

## Implementation Details

### Key Components

1. **API Client (`api.go`)**

   - Handles all interactions with HeadHunter API
   - Implements rate limiting and error handling
   - Uses MD5 hashing for duplicate detection

2. **Configuration (`config.go`)**

   - Manages application settings
   - Supports both command-line flags and environment variables

3. **MongoDB Storage (`mongo_storage.go`)**

   - Provides efficient data storage and retrieval
   - Implements in-memory caching for fast duplicate checks
   - Supports upsert operations

4. **Daily Runner (`run_daily.sh`)**

   - Automates daily data collection
   - Can optionally fetch historical data (30 days back)
   - Runs continuously with 24-hour intervals

5. **Logging (`logger.go`)**
   - Dual-channel logging (info and error)
   - Automatic log file rotation
   - Detailed timestamps and error context

### Performance Features

- Concurrent processing (10 goroutines by default)
- Retry mechanism for failed requests (3 attempts)
- Efficient memory usage with sync.Map for hash storage
- Batch processing of API results

## License

This project is licensed under the GPL-3.0 License - see the [LICENSE](LICENSE) file for details.

## Contribution

Contributions are welcome! Please open an issue or submit a pull request for any improvements.
