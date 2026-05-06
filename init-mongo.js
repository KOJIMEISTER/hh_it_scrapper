db = db.getSiblingDB("vacancy_db");

db.createCollection("vacancies");

db.vacancies.createIndex({ id: 1 }, { unique: true });
db.vacancies.createIndex(
  { description_hash: 1 },
  { unique: true, sparse: true }
);

db.vacancies.createIndex({ "key_skills.name": 1 });

const skillsPipeline = [
  {
    $match: {
      "key_skills.name": {
        $regex: /^(c\+\+|golang|go)$/i
      }
    }
  },
  {
    $unwind: "$key_skills"
  },
  {
    $match: {
      "key_skills.name": {
        $regex: /^(c\+\+|golang|go)$/i
      }
    }
  },
  {
    $project: {
      date_formatted: {
        $dateToString: {
          format: "%Y-%m-%d",
          date: { $dateFromString: { dateString: "$published_at" } },
          timezone: "Europe/Moscow"
        }
      },
      skill_normalized: {
        $cond: {
          if: { 
            $in: [ { $toLower: "$key_skills.name" }, ["go", "golang"] ] 
          },
          then: "golang",
          else: { $toLower: "$key_skills.name" }
        }
      }
    }
  },
  {
    $group: {
      _id: {
        date: "$date_formatted",
        skill: "$skill_normalized"
      },
      count: { $sum: 1 }
    }
  },
  {
    $project: {
      _id: 0,
      date: "$_id.date",
      skill: "$_id.skill",
      count: 1
    }
  },
  {
    $sort: {
      date: -1,
      skill: 1
    }
  }
];

db.createView(
  "vacancies_skills_daily_stats", 
  "vacancies", 
  skillsPipeline
);