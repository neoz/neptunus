# Neptunus

Neptunus is a data processing engine for consuming, transforming and producing events. Neptunus is based on data processing pipelines - a compositions of six types of plugins:
 - [Inputs](plugins/inputs/) consume events from external sources
 - [Processors](plugins/processors/) transform events
 - [Outputs](plugins/outputs/) produce events to external systems
 - [Filters](plugins/filters/) route events in pipeline by conditions
 - [Parsers](plugins/parsers/) convert raw data into events
 - [Serializers](plugins/serializers/) convert events into data formats for sending to external systems

# Configuration
Neptunus configuration has two parts - daemon config and pipelines.

See more in our [documentation](docs/CONFIGURATION.md).

# Getting Started
### Get help:
```
neptunus --help
```

### Run daemon:
```
neptunus run --config config.toml
```

### Test pipelines configuration:
```
neptunus test --config config.toml
```

### Get help about cli tool usage:
```
neptunus pipeline --help
```
