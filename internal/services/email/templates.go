package email

import (
	"bytes"
	"embed"
	_ "embed"
	"fmt"
	html "html/template"
	"strings"
	"text/template"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/shared"
	"go.uber.org/zap"
)

//go:embed generated
var templateFS embed.FS

var (
	passwordResetHTMLTemplate      = html.Must(html.ParseFS(templateFS, "generated/password_reset.html"))
	passwordResetTextTemplate      = template.Must(template.ParseFS(templateFS, "generated/password_reset.txt"))
	uploadNotificationHTMLTemplate = html.Must(html.ParseFS(templateFS, "generated/upload_notification.html"))
	uploadNotificationTextTemplate = template.Must(template.ParseFS(templateFS, "generated/upload_notification.txt"))
	secureLinkHTMLTemplate         = html.Must(html.ParseFS(templateFS, "generated/secure_link.html"))
	secureLinkTextTemplate         = template.Must(template.ParseFS(templateFS, "generated/secure_link.txt"))
	userInviteHTMLTemplate         = html.Must(html.ParseFS(templateFS, "generated/user_invite.html"))
	userInviteTextTemplate         = template.Must(template.ParseFS(templateFS, "generated/user_invite.txt"))
)

type uploadNotificationTemplateData struct {
	ClientName  string
	Files       []models.File
	ProductName string
	BaseURL     string
}

type passwordResetTemplateData struct {
	DisplayName string
	ResetLink   string
	Expiry      string
	ProductName string
}

type secureLinkTemplateData struct {
	ClientName  string
	AccessType  string
	LinkURL     string
	Expiry      string
	ProductName string
}

type userInviteTemplateData struct {
	DisplayName string
	InviteURL   string
	Expiry      string
	ProductName string
	WelcomeText string
}

func RenderUploadNotification(clientName string, files []models.File, baseURL, productName string) (subject, body, htmlBody string) {
	if productName == "" {
		productName = "ClientShare"
	}

	tplData := uploadNotificationTemplateData{
		ClientName:  clientName,
		Files:       files,
		BaseURL:     baseURL,
		ProductName: productName,
	}

	if len(files) == 1 {
		subject = fmt.Sprintf("%s: New file uploaded: %s", productName, files[0].Filename)
	} else {
		subject = fmt.Sprintf("%s: %d new files uploaded", productName, len(files))
	}
	body = renderUploadNotificationText(tplData)
	htmlBody = renderUploadNotificationHTML(tplData)
	return
}

func RenderDownloadNotification(clientName, fileName string) (subject, body string) {
	subject = fmt.Sprintf("File downloaded: %s", fileName)
	body = fmt.Sprintf("The file '%s' was downloaded for client '%s'.", fileName, clientName)
	return
}

func RenderPasswordReset(displayName, resetLink, productName string, expiry time.Duration) (subject, body, htmlBody string) {
	if productName == "" {
		productName = "ClientShare"
	}

	tplData := passwordResetTemplateData{
		DisplayName: displayName,
		ResetLink:   resetLink,
		Expiry:      expiry.String(),
		ProductName: productName,
	}

	subject = "Reset your password"
	body = renderPasswordResetText(tplData)
	htmlBody = renderPasswordResetHTML(tplData)
	return
}

func RenderSecureLinkEmail(clientName, accessType, linkURL, productName string, expiresAt time.Time) (subject, body, htmlBody string) {
	if productName == "" {
		productName = "ClientShare"
	}
	accessLabel := strings.ToLower(strings.TrimSpace(accessType))
	if accessLabel == "" {
		accessLabel = "read"
	}
	switch accessLabel {
	case "readwrite":
		accessLabel = "Read & Write"
	case "write":
		accessLabel = "Write"
	case "read":
		accessLabel = "Read"
	}

	formattedExpiry := expiresAt.Format("Jan 2, 2006")
	if expiresAt.IsZero() {
		formattedExpiry = "N/A"
	}

	tplData := secureLinkTemplateData{
		ClientName:  clientName,
		AccessType:  accessLabel,
		LinkURL:     linkURL,
		Expiry:      formattedExpiry,
		ProductName: productName,
	}

	subject = fmt.Sprintf("%s: Secure link ready", productName)
	body = renderSecureLinkText(tplData)
	htmlBody = renderSecureLinkHTML(tplData)
	return
}

func RenderUserInviteEmail(displayName, inviteURL, productName, welcomeText string, expiresAt time.Time) (subject, body, htmlBody string) {
	if productName == "" {
		productName = "ClientShare"
	}

	resolvedWelcomeText := strings.TrimSpace(welcomeText)
	if resolvedWelcomeText == "" {
		resolvedWelcomeText = shared.DefaultInviteWelcomeText
	}

	resolvedDisplayName := strings.TrimSpace(displayName)
	if resolvedDisplayName == "" {
		resolvedDisplayName = "there"
	}

	formattedExpiry := expiresAt.Format("Jan 2, 2006 3:04 PM MST")
	if expiresAt.IsZero() {
		formattedExpiry = "N/A"
	}

	tplData := userInviteTemplateData{
		DisplayName: resolvedDisplayName,
		InviteURL:   inviteURL,
		Expiry:      formattedExpiry,
		ProductName: productName,
		WelcomeText: resolvedWelcomeText,
	}

	subject = fmt.Sprintf("%s: Set up your account", productName)
	body = renderUserInviteText(tplData)
	htmlBody = renderUserInviteHTML(tplData)
	return
}

func renderPasswordResetHTML(data passwordResetTemplateData) string {
	var output bytes.Buffer
	if err := passwordResetHTMLTemplate.Execute(&output, data); err != nil {
		return fmt.Sprintf("<p>Hello %s,</p><p>We received a request to reset your password. Use the link below to set a new password.</p><p><a href=\"%s\">%s</a></p><p>This link expires in %s. If you did not request a password reset, you can ignore this email.</p>", data.DisplayName, data.ResetLink, data.ResetLink, data.Expiry)
	}
	return output.String()
}

func renderPasswordResetText(data passwordResetTemplateData) string {
	var output bytes.Buffer
	if err := passwordResetTextTemplate.Execute(&output, data); err != nil {
		return fmt.Sprintf("Hello %s,\n\nWe received a request to reset your password. Use the link below to set a new password.\n\n%s\n\nThis link expires in %s. If you did not request a password reset, you can ignore this email.\n", data.DisplayName, data.ResetLink, data.Expiry)
	}
	return output.String()
}

func renderUploadNotificationHTML(data uploadNotificationTemplateData) string {
	var output bytes.Buffer
	if err := uploadNotificationHTMLTemplate.Execute(&output, data); err != nil {
		fmt.Println(err)
		zap.L().Error("failed to render upload notification HTML template", zap.Error(err))
		firstFileName := ""
		if len(data.Files) > 0 {
			firstFileName = data.Files[0].Filename
		}
		return fmt.Sprintf("<p>A new file has been uploaded for client <strong>%s</strong>.</p><p><strong>File:</strong> %s</p><p>This is an automated notification from %s.</p>", data.ClientName, firstFileName, data.ProductName)
	}
	return output.String()
}

func renderUploadNotificationText(data uploadNotificationTemplateData) string {
	var output bytes.Buffer
	if err := uploadNotificationTextTemplate.Execute(&output, data); err != nil {
		fmt.Println(err)
		zap.L().Error("failed to render upload notification TEXT template", zap.Error(err))
		firstFileName := ""
		if len(data.Files) > 0 {
			firstFileName = data.Files[0].Filename
		}
		return fmt.Sprintf("%s\n\nNEW FILE UPLOADED\n\nA new file has been uploaded for client %s.\n\nFile: %s\n\nThis is an automated notification from %s.\n", data.ProductName, data.ClientName, firstFileName, data.ProductName)
	}
	return output.String()
}

func renderSecureLinkHTML(data secureLinkTemplateData) string {
	var output bytes.Buffer
	if err := secureLinkHTMLTemplate.Execute(&output, data); err != nil {
		zap.L().Error("failed to render secure link HTML template", zap.Error(err))
		return fmt.Sprintf("<p>A secure link to %s is ready.</p><p>Access level: %s</p><p><a href=\"%s\">%s</a></p><p>Expires on %s.</p>", data.ClientName, data.AccessType, data.LinkURL, data.LinkURL, data.Expiry)
	}
	return output.String()
}

func renderSecureLinkText(data secureLinkTemplateData) string {
	var output bytes.Buffer
	if err := secureLinkTextTemplate.Execute(&output, data); err != nil {
		zap.L().Error("failed to render secure link TEXT template", zap.Error(err))
		return fmt.Sprintf("SECURE LINK READY\n\nA secure link to %s is ready.\nAccess level: %s\nLink: %s\nExpires on %s.\n", data.ClientName, data.AccessType, data.LinkURL, data.Expiry)
	}
	return output.String()
}

func renderUserInviteHTML(data userInviteTemplateData) string {
	var output bytes.Buffer
	if err := userInviteHTMLTemplate.Execute(&output, data); err != nil {
		zap.L().Error("failed to render user invite HTML template", zap.Error(err))
		return fmt.Sprintf("<p>Hello %s,</p><p>You were invited to %s. %s</p><p>Set your password using this link:</p><p><a href=\"%s\">%s</a></p><p>This invite expires on %s.</p>", data.DisplayName, data.ProductName, data.WelcomeText, data.InviteURL, data.InviteURL, data.Expiry)
	}
	return output.String()
}

func renderUserInviteText(data userInviteTemplateData) string {
	var output bytes.Buffer
	if err := userInviteTextTemplate.Execute(&output, data); err != nil {
		zap.L().Error("failed to render user invite TEXT template", zap.Error(err))
		return fmt.Sprintf("INVITE TO %s\n\nHello %s,\n\n%s\n\nSet your password using this invite link:\n%s\n\nThis invite expires on %s.\n", data.ProductName, data.DisplayName, data.WelcomeText, data.InviteURL, data.Expiry)
	}
	return output.String()
}
