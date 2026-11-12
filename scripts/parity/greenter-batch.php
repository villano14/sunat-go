<?php
// Batch, withholding and despatch fixtures mirroring sunat-go's ubl goldens.
require getenv('GREENTER_AUTOLOAD') ?: exit("set GREENTER_AUTOLOAD to a vendor/autoload.php with greenter/lite\n");

use Greenter\Model\Client\Client;
use Greenter\Model\Company\{Company, Address};
use Greenter\Model\Sale\{Document, DetailAttribute};
use Greenter\Model\Summary\{Summary, SummaryDetail};
use Greenter\Model\Voided\{Voided, VoidedDetail, Reversion};
use Greenter\Model\Retention\{Retention, RetentionDetail, Payment, Exchange};
use Greenter\Model\Perception\{Perception, PerceptionDetail};
use Greenter\Model\Despatch\{Despatch, Shipment, Transportist, Direction, DespatchDetail, AdditionalDoc};
use Greenter\Xml\Builder\{SummaryBuilder, VoidedBuilder, RetentionBuilder, PerceptionBuilder, DespatchBuilder};

$out = $argv[1];
$tz = new DateTimeZone('America/Lima');
$at = fn(string $s) => new DateTime($s, $tz);
$batchCo = (new Company())->setRuc('20000000001')->setRazonSocial('EMPRESA & PRUEBA S.A.C.');

$sum = (new Summary())->setCorrelativo('1')->setFecGeneracion($at('2026-10-27'))->setFecResumen($at('2026-10-28 09:00'))
    ->setMoneda('PEN')->setCompany($batchCo)->setDetails([
        (new SummaryDetail())->setTipoDoc('03')->setSerieNro('B001-1')->setClienteTipo('1')->setClienteNro('12345678')->setEstado('1')
            ->setTotal(119.30)->setMtoOperGravadas(100)->setMtoOtrosCargos(1)->setMtoIGV(18)->setMtoIcbper(0.30)->setPorcentajeIgv(18),
        (new SummaryDetail())->setTipoDoc('07')->setSerieNro('BC01-7')->setClienteTipo('0')->setClienteNro('-')
            ->setDocReferencia((new Document())->setTipoDoc('03')->setNroDoc('B001-1'))->setEstado('1')
            ->setTotal(50)->setMtoOperExoneradas(50),
    ]);
file_put_contents("$out/summary-basic.xml", (new SummaryBuilder())->build($sum));

$vd = (new Voided())->setCorrelativo('2')->setFecGeneracion($at('2026-10-27'))->setFecComunicacion($at('2026-10-28 09:00'))
    ->setCompany($batchCo)->setDetails([
        (new VoidedDetail())->setTipoDoc('01')->setSerie('F001')->setCorrelativo('12')->setDesMotivoBaja('ERROR EN <DATOS> & MONTO'),
        (new VoidedDetail())->setTipoDoc('07')->setSerie('FC01')->setCorrelativo('3')->setDesMotivoBaja('ANULACION'),
    ]);
file_put_contents("$out/voided-basic.xml", (new VoidedBuilder())->build($vd));

$rr = (new Reversion())->setCorrelativo('3')->setFecGeneracion($at('2026-10-27'))->setFecComunicacion($at('2026-10-28 09:00'))
    ->setCompany($batchCo)->setDetails([
        (new VoidedDetail())->setTipoDoc('20')->setSerie('R001')->setCorrelativo('12')->setDesMotivoBaja('ERROR EN <TASA> & MONTO'),
        (new VoidedDetail())->setTipoDoc('40')->setSerie('P001')->setCorrelativo('4')->setDesMotivoBaja('REVERSION'),
    ]);
file_put_contents("$out/reversion-basic.xml", (new VoidedBuilder())->build($rr));

$whCo = (new Company())->setRuc('20000000001')->setRazonSocial('EMPRESA DE PRUEBA S.A.C.')->setNombreComercial('PRUEBA')
    ->setAddress((new Address())->setUbigueo('150101')->setDepartamento('LIMA')->setProvincia('LIMA')->setDistrito('LIMA')
        ->setDireccion('AV. LOS OLIVOS 123')->setCodigoPais('PE'));
$ret = (new Retention())->setSerie('R001')->setCorrelativo('1')->setFechaEmision($at('2026-10-27 13:05'))->setCompany($whCo)
    ->setProveedor((new Client())->setTipoDoc('6')->setNumDoc('20123456789')->setRznSocial('PROVEEDOR & ASOCIADOS S.A.'))
    ->setRegimen('01')->setTasa(3)->setImpRetenido(6)->setImpPagado(194)->setObservacion('OBSERVACION <1>')
    ->setDetails([
        (new RetentionDetail())->setTipoDoc('01')->setNumDoc('F001-1')->setFechaEmision($at('2026-10-20'))->setImpTotal(100)->setMoneda('USD')
            ->setPagos([(new Payment())->setMoneda('USD')->setImporte(100)->setFecha($at('2026-10-26'))])
            ->setFechaRetencion($at('2026-10-26'))->setImpRetenido(6)->setImpPagar(94)
            ->setTipoCambio((new Exchange())->setMonedaRef('USD')->setMonedaObj('PEN')->setFactor(3.75)->setFecha($at('2026-10-26'))),
        (new RetentionDetail())->setTipoDoc('01')->setNumDoc('F001-2')->setFechaEmision($at('2026-10-21'))->setImpTotal(100)->setMoneda('PEN')
            ->setPagos([(new Payment())->setMoneda('PEN')->setImporte(100)->setFecha($at('2026-10-26'))]),
    ]);
file_put_contents("$out/retention-basic.xml", (new RetentionBuilder())->build($ret));

$per = (new Perception())->setSerie('P001')->setCorrelativo('1')->setFechaEmision($at('2026-10-27 13:05'))->setCompany($whCo)
    ->setProveedor((new Client())->setTipoDoc('6')->setNumDoc('20123456789')->setRznSocial('CLIENTE & ASOCIADOS S.A.'))
    ->setRegimen('01')->setTasa(2)->setImpPercibido(2)->setImpCobrado(102)->setObservacion('OBSERVACION <1>')
    ->setDetails([(new PerceptionDetail())->setTipoDoc('01')->setNumDoc('F001-1')->setFechaEmision($at('2026-10-20'))->setImpTotal(100)->setMoneda('PEN')
        ->setCobros([(new Payment())->setMoneda('PEN')->setImporte(102)->setFecha($at('2026-10-25'))])
        ->setFechaPercepcion($at('2026-10-25'))->setImpPercibido(2)->setImpCobrar(102)]);
file_put_contents("$out/perception-basic.xml", (new PerceptionBuilder())->build($per));

$gre = (new Despatch())->setVersion('2022')->setTipoDoc('09')->setSerie('T001')->setCorrelativo('1')
    ->setFechaEmision($at('2026-11-05 09:30'))->setObservacion('ENTREGA <URGENTE> & FRAGIL')
    ->setCompany((new Company())->setRuc('20000000001')->setRazonSocial('EMPRESA DE PRUEBA S.A.C.'))
    ->setDestinatario((new Client())->setTipoDoc('6')->setNumDoc('20123456789')->setRznSocial('CLIENTE & ASOCIADOS S.A.'))
    ->setTercero((new Client())->setTipoDoc('6')->setNumDoc('20555555551')->setRznSocial('PROVEEDOR S.A.'))
    ->setComprador((new Client())->setTipoDoc('1')->setNumDoc('44004400')->setRznSocial('JUAN PEREZ'))
    ->setAddDocs([(new AdditionalDoc())->setTipoDesc('Factura')->setTipo('01')->setNro('F001-1')->setEmisor('20000000001')])
    ->setEnvio((new Shipment())->setCodTraslado('01')->setDesTraslado('VENTA')->setModTraslado('01')->setFecTraslado($at('2026-11-06'))
        ->setPesoTotal(12.5)->setUndPesoTotal('KGM')->setNumBultos(3)
        ->setTransportista((new Transportist())->setTipoDoc('6')->setNumDoc('20987654321')->setRznSocial('TRANSPORTES RAPIDOS S.A.C.')->setNroMtc('0000001'))
        ->setPartida((new Direction('150101', 'AV. LOS OLIVOS 123')))
        ->setLlegada((new Direction('150203', 'AV. ITALIA 456'))->setCodLocal('0001')->setRuc('20123456789')))
    ->setDetails([(new DespatchDetail())->setCodigo('P001')->setUnidad('NIU')->setDescripcion('PRODUCTO <UNO>')->setCantidad(2.5)
        ->setCodProdSunat('10101501')->setAtributos([(new DetailAttribute())->setCode('7000')->setName('Lote')->setValue('L-1')])]);
file_put_contents("$out/despatch-basic.xml", (new DespatchBuilder())->build($gre));
echo "ok\n";
