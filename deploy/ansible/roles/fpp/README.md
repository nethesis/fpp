# Ansible Role: fpp


Install and manage FPP (what the Fuck Push Proxy)

## Install

The role can be installed using [gilt](https://gilt.readthedocs.io/en/latest/)

On your ansible playbook directory, create the `gilt.yml` configuration file:

```yaml
- git: https://github.com/nethesis/fpp
  version: master
  files:
    - src: deploy/ansible/roles/fpp
      dst: roles/fpp
```

Install the role with:
```
gilt overlay
```

## Role Variables

| Name                  | Default Value                     | Description                               |
|-----------------------|-----------------------------------|-------------------------------------------|
| `fpp_version`         | 0.0.8                             | fpp version                               |
| `fpp_url`             | https://github.com/nethesis/fpp   | fpp repo url                              |
| `fpp_auth_service_url`| ""                                | External auth service, empty to disable   |
| `fpp_metrics_path`    | ""                                | secret path for metrics, empty to disable |
| `fpp_ping`            | True                              | Enbale `/ping` HTTP endpoint              |
| `brands`              | {}                                | dict of brand definition                  |


#### Traefik confguration
The ansible role uses Traefik for the HTTP routing. Traefik can be manually installed or via this ansible [role](https://github.com/Amygos/ansible-traefik)

Traefik static configuration compatible with the role:

```yaml
entryPoints:
  https:
    address: :443
providers:
  file:
    directory: /etc/traefik/conf.d
accessLog:
  format: json
metrics:
  prometheus:
    addEntryPointsLabels: true
    addServicesLabels: true
    manualRouting: true
ping:
  manualRouting: true
```

#### External authentication service

Using `fpp_auth_service_url` variable, the requests on `/send` and `/ping` endpoint can be authenticate, by forward it to an external
service using the [Traefik ForwardAuth middleware](https://doc.traefik.io/traefik/middlewares/http/forwardauth/)

#### Prometheus metrics

Fpp and Traefik expose application metrics using the Prometheus format on the `/metrics` endpoints.
The paths of the endpoints can be configured via the `fpp_metrics_path` variable:

* Traefik: `/<fpp_metrics_path>/metrics`
* Fpp: `/<brand_name>/<fpp_metrics_path>/metrics`

For security reasons, the `fpp_metrics_path` variable should be set to a random string, like an uuid.

#### Brands definition

The list of brands that fpp mange is defined into the dict `brand` variable, and the keys contain the brand configuration.
The name of the key is the name of the brand and the keys of the dict are the configuration for the brand:

* `http_port`: unique port to bind the fpp brand instance.
* `google_credentials_file`: local path to the Google application credentials JSON file.
* `apple_credentials_file`: local path to the Apple APNs p8 file.
* `apple_team_id`: Apple Team ID.
* `apple_key_id`: Apple Key ID.
* `apple_topic`: APNs topic.
* `apple_environment`: the environment for push notifications.
* `instance_token`: bearer token for the `/register` and `/deregister` endpoints.

```yaml
brands:
  example:
    http_port: 8081
    google_credentials_file: credentials.json
    apple_credentials_file: credentials.p8
    apple_team_id: 4tf7of1hqb
    apple_key_id: cit75cjk43
    apple_topic: com.example.MyApp
    apple_environment: production
    instance_token: 1cde32b5-fdc0-464d-86ee-6a91dab8fe27
```

HTTP route for the brands:

* `/<brand_name>/<fpp_metrics_path>/metrics`
* `/<brand_name>/register`
* `/<brand_name>/deregister`
* `/<brand_name>/send`

## Example Playbook

```yaml
- hosts: all
  become: true
  pre_tasks:
   - name: Disable SELinux
     ansible.posix.selinux:
       state: disabled
  roles:
    - role: amygos.traefik
      vars:
        traefik_static_configuration:
          entryPoints:
            https:
              address: :443
          providers:
            file:
              directory: /etc/traefik/conf.d
          accessLog:
            format: json
          metrics:
            prometheus:
              addEntryPointsLabels: true
              addServicesLabels: true
              manualRouting: true
          ping:
            manualRouting: true
    - role: fpp
      vars:
        brands:
          example:
            http_port: 8081
            google_credentials_file: credentials.json
            apple_credentials_file: credentials.p8
            apple_team_id: 4tf7of1hqb
            apple_key_id: cit75cjk43
            apple_topic: com.example.MyApp.voip
            apple_environment: production
            instance_token: 1cde32b5-fdc0-464d-86ee-6a91dab8fe27
```

## License

AGPL-3.0
