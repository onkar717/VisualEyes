# Security Policy

## Supported Versions

Use this section to tell people about which versions of your project are currently being supported with security updates.

| Version | Supported          |
| ------- | ------------------ |
| 1.0.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take the security of VisualEyes seriously. If you believe you have found a security vulnerability, please report it to us as described below.

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to security@visualeyes.dev (replace with your actual security contact).

You should receive a response within 48 hours. If for some reason you do not, please follow up via email to ensure we received your original message.

Please include the requested information listed below (as much as you can provide) to help us better understand the nature and scope of the possible issue:

* Type of issue (e.g. buffer overflow, SQL injection, cross-site scripting, etc.)
* Full paths of source file(s) related to the manifestation of the issue
* The location of the affected source code (tag/branch/commit or direct URL)
* Any special configuration required to reproduce the issue
* Step-by-step instructions to reproduce the issue
* Proof-of-concept or exploit code (if possible)
* Impact of the issue, including how an attacker might exploit it

## Preferred Languages

We prefer all communications to be in English.

## Security Best Practices

When deploying VisualEyes, follow these security best practices:

1. **Environment Variables**
   - Never commit .env files
   - Use secure secrets management in production
   - Rotate credentials regularly

2. **Database Security**
   - Use strong passwords
   - Enable SSL/TLS for database connections
   - Restrict database access to necessary IPs only

3. **API Security**
   - Use HTTPS for all API endpoints
   - Implement rate limiting
   - Validate all input data
   - Use proper authentication and authorization

4. **Kubernetes Deployment**
   - Use minimal RBAC permissions
   - Enable pod security policies
   - Keep Kubernetes version updated
   - Use network policies to restrict pod communication

5. **Docker Security**
   - Use official base images
   - Keep images updated
   - Run containers as non-root
   - Scan images for vulnerabilities

## Disclosure Policy

When we receive a security bug report, we will:

1. Confirm the problem and determine the affected versions.
2. Audit code to find any potential similar problems.
3. Prepare fixes for all still-maintained versions.
4. Release new versions and update the advisory.

## Comments on this Policy

If you have suggestions on how this process could be improved, please submit a pull request. 