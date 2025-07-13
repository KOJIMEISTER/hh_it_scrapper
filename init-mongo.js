db = db.getSiblingDB("vacancy_db");

db.createCollection("vacancies");

db.vacancies.createIndex({ id: 1 }, { unique: true });
db.vacancies.createIndex(
  { description_hash: 1 },
  { unique: true, sparse: true }
);
