# CLAUDE.md

This file provides guidance to Claude Code when working on the Resume Improvement Project.

## Project Context

This is a **private, personal career development repository** for improving a software engineering resume to target senior-level positions at companies like Datadog. The owner is an experienced software engineer with strong Ruby/Rails, Python, AWS, and Kubernetes background, seeking to showcase additional skills in Go, distributed systems, observability, and event streaming.

## Repository Purpose

**NOT** a codebase for production software. This is a **portfolio and planning repository** that includes:
- Task tracking and project management (TODO.md)
- Prototype projects to demonstrate technical skills
- Blog post drafts and technical writing
- Resume iterations optimized for specific roles
- Certification progress tracking

## Key Guidelines

### Development Philosophy
- **Pragmatic Prototyping**: Projects should be quick, demonstrable proofs of concept, not production-ready systems
- **Portfolio Quality**: Code should be clean, well-documented, and showcase best practices
- **Time-Boxed**: Most tasks target 1-8 hour completion windows (tight deadline: October 27, 2025)
- **Real Over Fake**: Focus on genuine achievements and learnings, not fabricated credentials

### Technology Focus Areas
1. **Go Development**: Build microservices, demonstrate concurrency, idiomatic Go patterns
2. **Event Streaming**: Kafka/RabbitMQ implementations, producer-consumer patterns
3. **Observability**: Metrics, logging, tracing (Prometheus, CloudWatch, Elasticsearch)
4. **Distributed Systems**: Scalability patterns, resilience, performance optimization
5. **AI/ML Integration**: Anomaly detection, predictive monitoring (scikit-learn, TensorFlow)

### Project Standards
- **README-Driven Development**: Every project subfolder needs clear documentation
- **Runnable Examples**: Include docker-compose or simple run instructions
- **Test Coverage**: Demonstrate TDD where appropriate (unit tests for Go/Python)
- **Architecture Decisions**: Document trade-offs and technology choices (e.g., "Why RabbitMQ over Kafka")

### Resume and Writing Guidelines
- **ATS Optimization**: Use keywords from target job descriptions (e.g., "distributed systems", "observability", "event-driven architecture")
- **Quantifiable Impact**: Include metrics where possible (e.g., "reduced incident response time by 40%")
- **Technical Writing**: Blog posts should be educational, not self-promotional; cite real tools/experiences
- **Authenticity**: Avoid buzzword bloat; focus on demonstrable skills

### Task Management
- **Atomic Commits**: Small, focused commits with descriptive messages (semantic prefixes: feat:, docs:, refactor:)
- **TODO Tracking**: Use TODO.md as single source of truth; check off completed items promptly
- **Prioritization**: Deadline-driven (October 21-27, 2025 for initial wave); high-impact tasks first
- **Feedback Loops**: Ask clarifying questions before starting complex implementations

## Common Development Commands

### Go Projects
```bash
# Initialize new Go module
go mod init github.com/username/project-name

# Run locally
go run main.go

# Build binary
go build -o bin/app

# Run tests
go test ./... -v

# Format and lint
go fmt ./...
go vet ./...
```

### Python Projects (Event Streaming, ML)
```bash
# Setup virtual environment
python3 -m venv venv
source venv/bin/activate

# Install dependencies
pip install -r requirements.txt

# Run application
python main.py

# Run tests
pytest tests/ -v

# Type checking and linting
mypy .
flake8 .
```

### Docker for Prototypes
```bash
# Build and run with docker-compose
docker-compose up --build

# Run specific service
docker-compose up kafka

# Clean up
docker-compose down -v
```

## File Structure Conventions

```
resume-improvements/
├── projects/
│   ├── go-log-stream/          # Each project is self-contained
│   │   ├── README.md           # Project-specific docs
│   │   ├── main.go
│   │   ├── go.mod
│   │   ├── docker-compose.yml  # Local dev environment
│   │   └── tests/
│   ├── rabbitmq-event-demo/
│   │   ├── README.md
│   │   ├── producer.py
│   │   ├── consumer.py
│   │   ├── requirements.txt
│   │   └── docker-compose.yml
│   └── ml-anomaly-detection/
│       ├── README.md
│       ├── model.py
│       ├── requirements.txt
│       └── notebooks/          # Jupyter notebooks for exploration
├── blog-posts/
│   ├── elasticsearch-tuning.md
│   └── kubernetes-best-practices.md
├── resume-versions/
│   ├── datadog-senior-engineer.pdf
│   └── generic-senior-swe.pdf
└── certifications/
    └── ckad-progress.md
```

## Important Project-Specific Notes

### Timeline Constraints
- **Hard Deadline**: October 27, 2025 (initial resume submission)
- **Aggressive Schedule**: 7 days from project start (October 20, 2025)
- **Prioritization Critical**: Focus on high-impact, quick-win tasks first

### Target Company Context (Datadog)
- **Scale**: Trillion events/day, distributed monitoring platform
- **Tech Stack**: Go, Python, Kafka, Kubernetes, Cassandra, Elasticsearch
- **Culture**: Data-driven, customer-focused, values technical depth and collaboration
- **Role Focus**: Senior engineers drive architecture, mentor juniors, improve observability at scale

### Owner's Strengths to Leverage
- Deep Rails/Ruby experience (can contrast with Go for learning demonstrations)
- AWS expertise (CloudWatch, S3, RDS, Kubernetes)
- Real-world production system management (AMT platform, payment processing)
- Cross-functional collaboration (Salesforce integration, SIS systems)

### Owner's Gap Areas to Address
- Limited public Go projects (need demonstrable code)
- Kafka/event streaming not prominently featured in resume
- AI/ML application to operations (emerging trend)
- Public visibility (blog posts, open-source contributions)

## Workflow for Claude

When starting a new task:
1. **Review TODO.md** for context and priority
2. **Clarify Scope**: Ask questions if requirements are ambiguous
3. **Research First**: Check existing similar projects, best practices
4. **Plan Incrementally**: Break into atomic steps (use TodoWrite tool)
5. **Document as You Go**: Update README files, add code comments
6. **Commit Atomically**: Small, descriptive commits
7. **Update TODO.md**: Check off completed items immediately

When writing code:
- Prefer idiomatic patterns (Go: channels, goroutines; Python: list comprehensions)
- Include error handling (Go: explicit error returns; Python: try/except)
- Write tests for core logic (aim for >70% coverage for portfolio projects)
- Add inline comments for non-obvious logic
- Use environment variables for configuration (never hardcode secrets)

When writing documentation:
- Use clear, concise language (assume reader is technical recruiter or engineer)
- Include "Why this matters" context (e.g., "Demonstrates handling 10K requests/sec")
- Provide runnable examples (copy-paste commands that work)
- Link to external resources for deeper dives

## Security and Privacy
- **No Sensitive Data**: This is a public portfolio repository (or will be shared)
- **Use Mock Data**: Fake logs, synthetic metrics, anonymized examples
- **No Real Credentials**: Use environment variables or dummy values
- **Clean Commit History**: No accidental credential commits

## Success Criteria

For each project/task, ensure:
- [ ] README explains what, why, and how to run
- [ ] Code is formatted and linted
- [ ] At least basic tests exist (if applicable)
- [ ] Commit messages are descriptive
- [ ] TODO.md is updated with completion status
- [ ] Demonstrates a skill relevant to target role

## Questions to Ask Before Starting

- What is the primary skill this task demonstrates?
- Can this be completed in the time budget (1-8 hours)?
- Does this align with Datadog's tech stack or values?
- Is this a genuine learning opportunity or just busywork?
- How will this appear to a hiring manager reviewing the portfolio?

---

*This file should be updated as the project evolves. When in doubt, prioritize pragmatism and demonstrable results over perfection.*
