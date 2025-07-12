db = db.getSiblingDB("vacancy_db");

db.createCollection("vacancies");

db.vacancies.createIndex({ id: 1 }, { unique: true });
