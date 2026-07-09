# Print Server

Multi-Printer Management Console

## Project Overview

Print Server is a multi-printer management console application developed in Go, supporting simultaneous management of multiple network printers and USB printers. The project adopts a layered architecture design, providing a solid foundation for extensibility across multiple brands, communication channels, and cloud integrations.

## Key Features

- **Multi-Printer Management**: Supports managing multiple printers simultaneously, including network and USB printers
- **Multi-Brand Support**: Compatible with major thermal printer brands such as Gprinter and Feie
- **ESC/POS Commands**: Supports the full ESC/POS command set, including text, barcodes, QR codes, and images
- **MQTT Integration**: Supports MQTT protocol for integration with cloud systems, enabling remote printing and status synchronization
- **Windows Desktop Interface**: Provides an intuitive Windows GUI for easy printer management
- **Print Job Management**: Supports job queuing, retry, and cancellation
- **Status Monitoring**: Real-time monitoring of printer online status and job status
- **Documentation Service**: Built-in API documentation server

## Technology Stack

- **Language**: Go
- **GUI Framework**: Walk (Windows GUI Library)
- **Communication Protocols**: TCP/IP, USB, MQTT
- **Printing Protocol**: ESC/POS

## Project Structure

```
├── main.go                      # Application entry point
├── go.mod / go.sum              # Go dependency management
├── build.ps1                    # Build script
├── app.manifest                 # Windows application manifest
│
├── internal/
│   ├── api/                     # API handling layer
│   │   ├── Print.go             # Print request handling
│   │   └── Print_test.go        # Test cases
│   │
│   ├── config/                  # Configuration management
│   │   └── Config.go            # Configuration file loading and saving
│   │
│   ├── docserver/               # API documentation server
│   │   ├── Server.go            # Documentation service implementation
│   │   └── apidoc.html          # API documentation page
│   │
│   ├── errcode/                 # Error code definitions
│   │   └── Errors.go            # Error types and codes
│   │
│   ├── escpos/                  # ESC/POS command set
│   │   ├── Barcode.go           # Barcode commands
│   │   ├── Buzzer.go            # Buzzer commands
│   │   ├── Command.go           # Basic command construction
│   │   ├── Finish.go            # Print completion commands
│   │   ├── Layout.go            # Layout calculation
│   │   ├── QRCode.go            # QR code commands
│   │   ├── Raster.go            # Raster graphics
│   │   ├── Receipt.go           # Receipt templates
│   │   └── Reprint.go           # Reprint commands
│   │
│   ├── instance/                # Single-instance control
│   │   └── SingleInstance.go    # Ensures single-instance execution
│   │
│   ├── layout/                  # Print content layout
│   │   ├── Element.go           # Layout element definitions
│   │   ├── Graphics.go          # Graphics rendering (QR codes, barcodes, images)
│   │   ├── Render.go            # Rendering engine
│   │   ├── Sample.go            # Sample content
│   │   ├── Table.go             # Table rendering
│   │   └── Text.go              # Text rendering
│   │
│   ├── logger/                  # Logging module
│   │   └── Logger.go            # Log recording
│   │
│   ├── model/                   # Data models
│   │   ├── Job.go               # Print job model
│   │   ├── Printer.go           # Printer model
│   │   └── Settings.go          # Settings model
│   │
│   ├── mqtt/                    # MQTT client
│   │   ├── Client.go            # MQTT connection management
│   │   └── Report.go            # Status reporting
│   │
│   ├── printsvc/                # Print service
│   │   ├── Events.go            # Print events
│   │   ├── Service.go           # Print job scheduling
│   │   └── Status.go            # Status queries
│   │
│   ├── transport/               # Transport layer
│   │   ├── Ping.go              # ICMP Ping detection
│   │   ├── Printer.go           # Printer interface definition
│   │   ├── Spooler.go           # Windows spooler
│   │   ├── SpoolerStatus.go     # Spooler status query
│   │   ├── Status.go            # Printer status
│   │   └── Tcp.go               # TCP connection
│   │
│   ├── ui/                      # User interface
│   │   ├── AddWizard.go         # Add printer wizard
│   │   ├── AlertWindow.go       # Alert window
│   │   ├── App.go               # Main application window
│   │   ├── Icon.go              # Application icon
│   │   ├── JobsView.go          # Print jobs view
│   │   ├── JsonTest.go          # JSON testing tool
│   │   ├── Notify.go            # Notification functionality
│   │   ├── OnlineDebounce.go    # Online status debouncing
│   │   ├── PrintersView.go      # Printers list view
│   │   ├── Properties.go        # Property editor
│   │   ├── SettingsView.go      # Settings page
│   │   ├── StatusMonitor.go     # Status monitoring
│   │   ├── Tables.go            # Table component
│   │   └── Tray.go              # System tray
│   │
│   ├── util/                    # Utility functions
│   │   └── Encoding.go          # Encoding conversion (GB18030)
│   │
│   └── assets/                  # Static assets
│       └── app.png             # Application icon
```

## Functional Overview

### Printer Management

- Supports adding, editing, and deleting printers
- Supports both network printers (TCP/IP) and USB printers
- Supports multiple brands: Gprinter, Feie, etc.
- Real-time monitoring of printer status

### Printing Functionality

- Supports print content definition in JSON format
- Supports elements such as text, tables, QR codes, barcodes, and images
- Supports custom paper widths (e.g., 58mm, 80mm)
- Supports alignment, font size, bold, and other styling options

### MQTT Cloud Integration

- Supports MQTT protocol connection to cloud servers
- Reports printer status to the cloud
- Receives print jobs from the cloud

### API Documentation

Built-in API documentation server providing complete print interface specifications.

## Build and Run

### Prerequisites

- Go 1.18+
- Windows 10/11
- GCC (for CGO)

### Build Steps

```powershell
# Build using PowerShell
./build.ps1
```

### Run

After building, launch the generated executable file to start the application.

## Configuration

Configuration files are saved by default in the program’s root directory and include:

- Printer list configuration
- MQTT connection settings
- API documentation server settings

## License

This project is intended solely for learning and reference purposes.