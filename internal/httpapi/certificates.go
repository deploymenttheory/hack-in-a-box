package httpapi

import (
	"bytes"
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/certificates"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) certificateOwner() certificates.Service {
	s := h.Certificates
	if s.Pool == nil {
		s.Pool = h.Pool
	}
	return s
}

func (h handlers) GetCertificateSettings(
	ctx context.Context,
	r contract.GetCertificateSettingsRequestObject,
) (contract.GetCertificateSettingsResponseObject, error) {
	v, err := h.certificateOwner().Settings(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("certificate API: %w", err)
	}
	return convertDTO[contract.GetCertificateSettings200JSONResponse](v)
}

func (h handlers) ConfigureCertificates(
	ctx context.Context,
	r contract.ConfigureCertificatesRequestObject,
) (contract.ConfigureCertificatesResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	c, err := convertDTO[certificates.SettingsCommand](r.Body)
	if err != nil {
		return nil, fmt.Errorf("certificate API: %w", err)
	}
	v, err := h.certificateOwner().Configure(ctx, actor(ctx).ID, r.EventId, c)
	if err != nil {
		return nil, fmt.Errorf("certificate API: %w", err)
	}
	return convertDTO[contract.ConfigureCertificates200JSONResponse](v)
}

func (h handlers) PreviewCertificateEligibility(
	ctx context.Context,
	r contract.PreviewCertificateEligibilityRequestObject,
) (contract.PreviewCertificateEligibilityResponseObject, error) {
	target := certificates.Target{PersonID: r.Params.PersonId, TemplateID: r.Params.TemplateId}
	if r.Params.PublicationId != nil {
		target.PublicationID = *r.Params.PublicationId
	}
	if r.Params.PrizeId != nil {
		target.PrizeID = *r.Params.PrizeId
	}
	v, err := h.certificateOwner().Preview(ctx, actor(ctx).ID, r.EventId, target)
	if err != nil {
		return nil, fmt.Errorf("certificate API: %w", err)
	}
	return convertDTO[contract.PreviewCertificateEligibility200JSONResponse](v)
}

func (h handlers) GetCertificate(
	ctx context.Context,
	r contract.GetCertificateRequestObject,
) (contract.GetCertificateResponseObject, error) {
	v, err := h.certificateOwner().Read(ctx, actor(ctx).ID, r.EventId, r.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("certificate API: %w", err)
	}
	return convertDTO[contract.GetCertificate200JSONResponse](v)
}

func (h handlers) ChangeCertificate(
	ctx context.Context,
	r contract.ChangeCertificateRequestObject,
) (contract.ChangeCertificateResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	c, err := convertDTO[certificates.Command](r.Body)
	if err != nil {
		return nil, fmt.Errorf("certificate API: %w", err)
	}
	v, err := h.certificateOwner().Change(ctx, actor(ctx).ID, r.EventId, r.CertificateId, c)
	if err != nil {
		return nil, fmt.Errorf("certificate API: %w", err)
	}
	return convertDTO[contract.ChangeCertificate200JSONResponse](v)
}

func (h handlers) PrintCertificate(
	ctx context.Context,
	r contract.PrintCertificateRequestObject,
) (contract.PrintCertificateResponseObject, error) {
	body, err := h.certificateOwner().Print(ctx, actor(ctx).ID, r.EventId, r.CertificateId)
	if err != nil {
		return nil, fmt.Errorf("certificate API: %w", err)
	}
	disposition := "attachment; filename=certificate-" + r.CertificateId.String() + ".html"
	policy := "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'"
	return contract.PrintCertificate200TexthtmlResponse{
		Body:          bytes.NewReader(body),
		ContentLength: int64(len(body)),
		Headers: contract.PrintCertificate200ResponseHeaders{
			ContentDisposition:    &disposition,
			ContentSecurityPolicy: &policy,
		},
	}, nil
}
